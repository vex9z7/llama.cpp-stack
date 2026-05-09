package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Proxy struct {
	Client *http.Client
}

var hopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

func (p Proxy) ForwardBytes(ctx context.Context, w http.ResponseWriter, r *http.Request, backendURL string, body []byte) error {
	base, err := url.Parse(strings.TrimRight(backendURL, "/"))
	if err != nil {
		return err
	}
	target := *base
	target.Path = joinPath(base.Path, r.URL.Path)
	target.RawQuery = r.URL.RawQuery
	up, err := http.NewRequestWithContext(ctx, r.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	up.ContentLength = int64(len(body))
	copyHeaders(up.Header, r.Header)
	up.Host = base.Host
	resp, err := p.http().Do(up)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	return copyFlush(w, resp.Body)
}

func (p Proxy) http() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: 0}
}

func copyHeaders(dst, src http.Header) {
	for k, vals := range src {
		if hopHeaders[strings.ToLower(k)] {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func copyFlush(w http.ResponseWriter, src io.Reader) error {
	buf := make([]byte, 32*1024)
	flusher, _ := w.(http.Flusher)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				return nil
			}
			return fmt.Errorf("proxy read: %w", rerr)
		}
	}
}

func joinPath(a, b string) string {
	if a == "" || a == "/" {
		return b
	}
	return strings.TrimRight(a, "/") + "/" + strings.TrimLeft(b, "/")
}
