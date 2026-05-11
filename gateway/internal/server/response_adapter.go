package server

import (
	"io"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/apiadapter"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/proxy"
)

func (a *App) writeUpstreamResponse(ctx huma.Context, resp *http.Response, model string) {
	appendResponseHeaders(ctx, resp.Header)
	if resp.Header.Get("Content-Type") == "" {
		ctx.SetHeader("Content-Type", "application/json; charset=utf-8")
	}
	ctx.SetStatus(resp.StatusCode)

	if shouldAdapt(ctx.URL().Path, resp) {
		if isEventStream(resp.Header.Get("Content-Type")) {
			if ctx.URL().Path == "/v1/responses" {
				if err := copyResponsesSSE(ctx.BodyWriter(), resp.Body); err != nil {
					a.log.Warn("responses SSE copy failed", "model", model, "error", err)
				}
				return
			}
			if err := proxy.CopyFlush(ctx.BodyWriter(), resp.Body); err != nil {
				a.log.Warn("stream proxy copy failed", "model", model, "error", err)
			}
			return
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			a.log.Warn("upstream body read failed", "model", model, "path", ctx.URL().Path, "error", err)
			return
		}
		body, err = adaptBody(ctx.URL().Path, body)
		if err != nil {
			a.log.Warn("typed adaptation failed", "model", model, "path", ctx.URL().Path, "error", err)
		}
		_, _ = ctx.BodyWriter().Write(body)
		return
	}

	if err := proxy.CopyFlush(ctx.BodyWriter(), resp.Body); err != nil {
		a.log.Warn("proxy copy failed", "model", model, "error", err)
	}
}

func shouldAdapt(path string, resp *http.Response) bool {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	switch path {
	case "/v1/chat/completions", "/v1/completions", "/v1/responses", "/v1/embeddings":
		return true
	default:
		return false
	}
}

func adaptBody(path string, body []byte) ([]byte, error) {
	switch path {
	case "/v1/chat/completions":
		return apiadapter.AdaptChatCompletionBody(body)
	case "/v1/completions":
		return apiadapter.AdaptCompletionBody(body)
	case "/v1/responses":
		return apiadapter.AdaptResponsesBody(body)
	case "/v1/embeddings":
		return apiadapter.AdaptEmbeddingBody(body)
	default:
		return body, nil
	}
}

func isEventStream(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}
