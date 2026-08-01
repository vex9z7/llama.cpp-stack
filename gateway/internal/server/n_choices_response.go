package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/proxy"
)

func (a *App) writeNChoiceResponse(ctx huma.Context, model string, headers http.Header, req nChoiceRequest) {
	if req.Stream {
		a.writeNChoiceStreamResponse(ctx, model, headers, req)
		return
	}
	bodies := make([][]byte, 0, req.N)
	var firstHeader http.Header
	for _, upstreamBody := range req.Bodies {
		resp, err := a.proxy.Do(ctx.Context(), ctx.Method(), ctx.URL().Path, ctx.URL().RawQuery, headers, a.manager.RouterBaseURL(), upstreamBody)
		if err != nil {
			a.log.Warn("proxy failed", "model", model, "error", err)
			if proxy.IsResponseHeaderTimeout(err) {
				a.writeOpenAIError(ctx, http.StatusGatewayTimeout, "upstream_error", "response_header_timeout", err.Error())
				return
			}
			a.writeOpenAIError(ctx, http.StatusServiceUnavailable, "upstream_error", "router_unavailable", err.Error())
			return
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			a.writeUpstreamResponse(ctx, resp, model)
			_ = resp.Body.Close()
			return
		}
		if firstHeader == nil {
			firstHeader = resp.Header.Clone()
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			a.log.Warn("upstream body read failed", "model", model, "path", ctx.URL().Path, "error", err)
			a.writeOpenAIError(ctx, http.StatusServiceUnavailable, "upstream_error", "upstream_read_failed", err.Error())
			return
		}
		bodies = append(bodies, body)
	}

	body, err := aggregateNChoiceBodies(bodies)
	if err != nil {
		a.log.Warn("n-choice aggregation failed", "model", model, "path", ctx.URL().Path, "error", err)
		a.writeOpenAIError(ctx, http.StatusServiceUnavailable, "upstream_error", "n_choice_aggregation_failed", err.Error())
		return
	}
	body, err = adaptBody(ctx.URL().Path, body)
	if err != nil {
		a.log.Warn("typed adaptation failed", "model", model, "path", ctx.URL().Path, "error", err)
	}
	appendResponseHeaders(ctx, firstHeader)
	ctx.SetHeader("Content-Type", "application/json; charset=utf-8")
	ctx.SetStatus(http.StatusOK)
	if _, err := ctx.BodyWriter().Write(body); err != nil {
		a.log.Debug("response body write failed", "model", model, "path", ctx.URL().Path, "error", err)
	}
}

func (a *App) writeNChoiceStreamResponse(ctx huma.Context, model string, headers http.Header, req nChoiceRequest) {
	started := false
	flusher, _ := ctx.BodyWriter().(http.Flusher)
	for i, upstreamBody := range req.Bodies {
		resp, err := a.proxy.Do(ctx.Context(), ctx.Method(), ctx.URL().Path, ctx.URL().RawQuery, headers, a.manager.RouterBaseURL(), upstreamBody)
		if err != nil {
			a.log.Warn("proxy failed", "model", model, "error", err)
			if !started {
				if proxy.IsResponseHeaderTimeout(err) {
					a.writeOpenAIError(ctx, http.StatusGatewayTimeout, "upstream_error", "response_header_timeout", err.Error())
					return
				}
				a.writeOpenAIError(ctx, http.StatusServiceUnavailable, "upstream_error", "router_unavailable", err.Error())
				return
			}
			_ = writeSSEError(ctx.BodyWriter(), err.Error())
			if flusher != nil {
				flusher.Flush()
			}
			return
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if !started {
				a.writeUpstreamResponse(ctx, resp, model)
				_ = resp.Body.Close()
				return
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			_ = writeSSEError(ctx.BodyWriter(), string(body))
			if flusher != nil {
				flusher.Flush()
			}
			return
		}

		if !started {
			appendResponseHeaders(ctx, resp.Header)
			ctx.SetHeader("Content-Type", "text/event-stream; charset=utf-8")
			ctx.SetStatus(http.StatusOK)
			started = true
		}

		err = copyNChoiceSSE(ctx.BodyWriter(), resp.Body, i, i == len(req.Bodies)-1)
		_ = resp.Body.Close()
		if flusher != nil {
			flusher.Flush()
		}
		if err != nil {
			a.log.Warn("n-choice SSE copy failed", "model", model, "path", ctx.URL().Path, "error", err)
			_ = writeSSEError(ctx.BodyWriter(), err.Error())
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
	}
}

func writeSSEError(w io.Writer, message string) error {
	payload := map[string]any{"error": map[string]any{"message": message, "type": "upstream_error"}}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = w.Write(append(append([]byte("event: error\ndata: "), body...), []byte("\n\n")...))
	return err
}
