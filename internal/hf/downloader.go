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

const (
	CodeRepoNotFound   = "hf_repo_not_found"
	CodeUnauthorized   = "hf_unauthorized"
	CodeForbidden      = "hf_forbidden"
	CodeListFailed     = "hf_list_failed"
	CodeNoMatchingFile = "hf_no_matching_file"
	CodeAmbiguousFiles = "hf_ambiguous_files"
	CodeSplitGGUF      = "hf_split_gguf"
	CodeDownloadFailed = "hf_download_failed"
	CodeEmptyDownload  = "hf_empty_download"
	CodeFileNotFound   = "hf_file_not_found"
)

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "hf_error"
}

func hferr(code, msg string, cause error) error {
	return &Error{Code: code, Message: msg, Cause: cause}
}

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
		return nil, hferr(CodeListFailed, "huggingface list files request failed", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, hferr(CodeRepoNotFound, fmt.Sprintf("huggingface repo not found: %s", repo), nil)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, hferr(CodeUnauthorized, "huggingface repo requires authentication or token is invalid", nil)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, hferr(CodeForbidden, "huggingface repo is gated/private or token lacks access", nil)
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, hferr(CodeListFailed, fmt.Sprintf("huggingface list files failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body))), nil)
	}
	var entries []treeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, hferr(CodeListFailed, "huggingface list files response was not valid JSON", err)
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
				if strings.Contains(strings.ToLower(path.Base(f)), "-of-") {
					return "", hferr(CodeSplitGGUF, fmt.Sprintf("configured file %q appears to be a split GGUF; pin a single-file GGUF repo or exact first shard support later", f), nil)
				}
				return f, nil
			}
		}
		return "", hferr(CodeFileNotFound, fmt.Sprintf("file %q not found in %s", m.File, m.Repo), nil)
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
		return "", hferr(CodeNoMatchingFile, fmt.Sprintf("no GGUF file matches %q in %s", pattern, m.Repo), nil)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		si := score(matches[i])
		sj := score(matches[j])
		if si != sj {
			return si < sj
		}
		return matches[i] < matches[j]
	})
	bestScore := score(matches[0])
	best := make([]string, 0, len(matches))
	for _, f := range matches {
		if score(f) == bestScore {
			best = append(best, f)
		}
	}
	if len(best) > 1 {
		return "", hferr(CodeAmbiguousFiles, fmt.Sprintf("multiple GGUF files match %q in %s: %s; set file or pattern in catalog", pattern, m.Repo, strings.Join(best, ", ")), nil)
	}
	if strings.Contains(strings.ToLower(path.Base(best[0])), "-of-") {
		return "", hferr(CodeSplitGGUF, fmt.Sprintf("matched file %q appears to be a split GGUF; use a single-file repo or pin a supported file", best[0]), nil)
	}
	return best[0], nil
}

func score(f string) int {
	b := strings.ToLower(path.Base(f))
	s := 0
	for _, bad := range []string{"mmproj", "imatrix", "vision"} {
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
		return hferr(CodeDownloadFailed, "huggingface download request failed", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		return hferr(CodeUnauthorized, "huggingface download requires authentication or token is invalid", nil)
	}
	if resp.StatusCode == http.StatusForbidden {
		return hferr(CodeForbidden, "huggingface download is gated/private or token lacks access", nil)
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return hferr(CodeDownloadFailed, fmt.Sprintf("huggingface download failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body))), nil)
	}
	tmp := stable + ".tmp"
	_ = os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return hferr(CodeDownloadFailed, "failed while writing downloaded model", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return hferr(CodeDownloadFailed, "failed closing downloaded model file", closeErr)
	}
	if info, err := os.Stat(tmp); err != nil || info.Size() == 0 {
		_ = os.Remove(tmp)
		if err != nil {
			return err
		}
		return hferr(CodeEmptyDownload, "downloaded file is empty", nil)
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
