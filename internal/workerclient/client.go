package workerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	ID      string
	BaseURL string
	HTTP    *http.Client
}

type Status struct {
	ID            string `json:"id"`
	State         string `json:"state"`
	ModelRef      string `json:"model_ref,omitempty"`
	ModelPath     string `json:"model_path,omitempty"`
	ModelName     string `json:"model_name,omitempty"`
	InferenceURL  string `json:"inference_url,omitempty"`
	PID           int    `json:"pid,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	LoadedAt      string `json:"loaded_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	StderrTail    string `json:"stderr_tail,omitempty"`
	Embeddings    bool   `json:"embeddings,omitempty"`
	LastExitError string `json:"last_exit_error,omitempty"`
}

type LoadRequest struct {
	ModelRef    string `json:"model_ref"`
	ModelPath   string `json:"model_path"`
	ModelName   string `json:"model_name"`
	CtxSize     int    `json:"ctx_size"`
	Parallel    int    `json:"parallel"`
	ThreadsHTTP int    `json:"threads_http"`
	NGPULayers  int    `json:"n_gpu_layers"`
	ExtraArgs   string `json:"extra_args"`
	TimeoutSec  int    `json:"timeout_seconds"`
	Embeddings  bool   `json:"embeddings"`
}

type LoadResponse struct {
	Status       string `json:"status"`
	Worker       string `json:"worker"`
	InferenceURL string `json:"inference_url"`
}

func New(baseURL string) Client {
	baseURL = strings.TrimRight(baseURL, "/")
	id := baseURL
	if i := strings.LastIndex(strings.TrimPrefix(baseURL, "http://"), ":"); i > 0 {
		withoutScheme := strings.TrimPrefix(strings.TrimPrefix(baseURL, "http://"), "https://")
		id = withoutScheme[:i]
	}
	return Client{ID: id, BaseURL: baseURL, HTTP: &http.Client{Timeout: 0}}
}

func (c Client) Status(ctx context.Context) (Status, error) {
	var s Status
	if err := c.doJSON(ctx, http.MethodGet, "/worker/status", nil, &s); err != nil {
		return s, err
	}
	if s.ID == "" {
		s.ID = c.ID
	}
	return s, nil
}

func (c Client) Load(ctx context.Context, req LoadRequest) (LoadResponse, error) {
	var out LoadResponse
	if err := c.doJSON(ctx, http.MethodPost, "/worker/load", req, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c Client) Unload(ctx context.Context, force bool) error {
	body := map[string]bool{"force": force}
	return c.doJSON(ctx, http.MethodPost, "/worker/unload", body, nil)
}

func (c Client) doJSON(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("worker %s %s failed: status=%d body=%s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}
