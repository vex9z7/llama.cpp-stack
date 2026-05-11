package routerclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type ModelsResponse struct {
	Object string        `json:"object"`
	Data   []ModelRecord `json:"data"`
}

type ModelRecord struct {
	ID      string          `json:"id"`
	Aliases []string        `json:"aliases,omitempty"`
	Object  string          `json:"object"`
	Status  json.RawMessage `json:"status,omitempty"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

func New(base string) Client { return Client{BaseURL: strings.TrimRight(base, "/")} }

func (c Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("router health status %d", resp.StatusCode)
	}
	return nil
}

func (c Client) Models(ctx context.Context, reload bool) (ModelsResponse, error) {
	u := c.BaseURL + "/models"
	if reload {
		u += "?reload=1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ModelsResponse{}, err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return ModelsResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ModelsResponse{}, fmt.Errorf("router models status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ModelsResponse{}, err
	}
	return out, nil
}

var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	},
}

func (c Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTPClient
}
