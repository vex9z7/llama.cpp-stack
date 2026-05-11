package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Proxy struct{ Client *http.Client }

var hopHeaders = map[string]bool{
	"connection": true, "content-length": true, "keep-alive": true, "proxy-authenticate": true, "proxy-authorization": true,
	"te": true, "trailer": true, "transfer-encoding": true, "upgrade": true,
}

func IsHopHeader(name string) bool { return hopHeaders[strings.ToLower(name)] }

func (p Proxy) Do(ctx context.Context, method, reqPath, rawQuery string, headers http.Header, backendURL string, body []byte) (*http.Response, error) {
	base, err := url.Parse(strings.TrimRight(backendURL, "/"))
	if err != nil {
		return nil, err
	}
	target := *base
	target.Path = joinPath(base.Path, reqPath)
	target.RawQuery = rawQuery
	up, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	up.ContentLength = int64(len(body))
	copyHeaders(up.Header, headers)
	up.Host = base.Host
	return p.http().Do(up)
}

var defaultHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	},
}

func (p Proxy) http() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return defaultHTTPClient
}

func copyHeaders(dst, src http.Header) {
	for k, vals := range src {
		if IsHopHeader(k) {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func CopyFlush(w io.Writer, src io.Reader) error {
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
