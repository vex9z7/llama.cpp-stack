package hf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vex9z7/llama.cpp-stack/internal/catalog"
)

type Downloader struct {
	Endpoint  string
	Token     string
	ModelsDir string
	Client    *http.Client
}

type treeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func (d *Downloader) Ensure(ctx context.Context, m catalog.Model) (string, error) {
	stable := m.StablePath(d.ModelsDir)
	if fileExists(stable) {
		return stable, nil
	}
	if err := os.MkdirAll(filepath.Dir(stable), 0o755); err != nil {
		return "", err
	}
	files, err := d.listFiles(ctx, m.Repo)
	if err != nil {
		return "", err
	}
	chosen, err := chooseFile(files, m)
	if err != nil {
		return "", err
	}
	if err := d.download(ctx, m.Repo, chosen, stable); err != nil {
		return "", err
	}
	return stable, nil
}

func (d *Downloader) listFiles(ctx context.Context, repo string) ([]string, error) {
	endpoint := strings.TrimRight(d.Endpoint, "/")
	apiURL := endpoint + "/api/models/" + repo + "/tree/main?recursive=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	d.addAuth(req)
	resp, err := d.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("huggingface repo not found: %s", repo)
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("huggingface list files failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var entries []treeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type == "file" || e.Type == "" {
			files = append(files, e.Path)
		}
	}
	sort.Strings(files)
	return files, nil
}

func chooseFile(files []string, m catalog.Model) (string, error) {
	if m.File != "" {
		for _, f := range files {
			if f == m.File || path.Base(f) == m.File {
				return f, nil
			}
		}
		return "", fmt.Errorf("file %q not found in %s", m.File, m.Repo)
	}
	pattern := m.GlobPattern()
	matches := make([]string, 0)
	for _, f := range files {
		base := path.Base(f)
		okBase, _ := path.Match(pattern, base)
		okPath, _ := path.Match(pattern, f)
		if strings.HasSuffix(strings.ToLower(base), ".gguf") && (okBase || okPath) {
			matches = append(matches, f)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no GGUF file matches %q in %s", pattern, m.Repo)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		// Prefer normal model files over mmproj/imatrix metadata when both match.
		si := score(matches[i])
		sj := score(matches[j])
		if si != sj {
			return si < sj
		}
		return matches[i] < matches[j]
	})
	return matches[0], nil
}

func score(f string) int {
	b := strings.ToLower(path.Base(f))
	s := 0
	for _, bad := range []string{"mmproj", "imatrix", "vision", "mmproj"} {
		if strings.Contains(b, bad) {
			s += 10
		}
	}
	return s
}

func (d *Downloader) download(ctx context.Context, repo, file, stable string) error {
	endpoint := strings.TrimRight(d.Endpoint, "/")
	parts := strings.Split(file, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	dlURL := endpoint + "/" + repo + "/resolve/main/" + strings.Join(parts, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}
	d.addAuth(req)
	resp, err := d.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("huggingface download failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	tmp := stable + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if info, err := os.Stat(tmp); err != nil || info.Size() == 0 {
		_ = os.Remove(tmp)
		if err != nil {
			return err
		}
		return errors.New("downloaded file is empty")
	}
	return os.Rename(tmp, stable)
}

func (d *Downloader) addAuth(req *http.Request) {
	if d.Token != "" {
		req.Header.Set("Authorization", "Bearer "+d.Token)
	}
}

func (d *Downloader) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return &http.Client{Timeout: 0, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 60 * time.Second}}
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Size() > 0
}
