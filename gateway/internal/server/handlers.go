package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/openaiapi"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/proxy"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/routermanager"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Router  string `json:"router"`
}

func (a *App) humaHealth(ctx huma.Context) {
	resp := healthResponse{Status: "ok", Service: "llama.cpp-stack-gateway", Router: "ok"}
	if err := a.manager.Health(ctx.Context()); err != nil {
		resp.Status = "degraded"
		resp.Router = "unavailable"
	}
	a.writeJSON(ctx, http.StatusOK, resp)
}

func (a *App) humaModels(ctx huma.Context) {
	models := a.manager.ListModels(ctx.Context())
	data := make([]openaiapi.Model, 0, len(models))
	for _, m := range models {
		data = append(data, openaiapi.Model{
			Id:      m.ID,
			Object:  m.Object,
			OwnedBy: m.OwnedBy,
			Meta: openaiapi.ModelMeta{
				Downloaded:   m.Downloaded,
				RouterStatus: stringPtr(m.RouterStatus),
				Running:      m.Running,
				ColdStart:    m.ColdStart,
				Repo:         m.Repo,
				Quant:        m.Quant,
				Kind:         stringPtr(m.Kind),
			},
		})
	}
	a.writeJSON(ctx, http.StatusOK, openaiapi.ModelList{Object: "list", Data: data})
}

func (a *App) humaInference(ctx huma.Context) {
	body, err := readLimited(ctx.BodyReader(), maxInferenceBodyBytes)
	if err != nil {
		a.writeOpenAIError(ctx, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return
	}
	var req openaiapi.ModelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		a.writeOpenAIError(ctx, http.StatusBadRequest, "invalid_request_error", "invalid_json", "request body must be JSON")
		return
	}
	if req.Model == "" {
		a.writeOpenAIError(ctx, http.StatusBadRequest, "invalid_request_error", "missing_model", "request body must include model")
		return
	}
	if err := a.manager.EnsureAvailable(ctx.Context(), req.Model, requiredKind(ctx.URL().Path)); err != nil {
		a.writeEnsureError(ctx, req.Model, err)
		return
	}
	headers := http.Header{}
	ctx.EachHeader(func(name, value string) { headers.Add(name, value) })
	upstreamBody, err := adaptRequestBody(ctx.URL().Path, body)
	if err != nil {
		a.writeOpenAIError(ctx, http.StatusBadRequest, "invalid_request_error", "invalid_request", err.Error())
		return
	}
	resp, err := a.proxy.Do(ctx.Context(), ctx.Method(), ctx.URL().Path, ctx.URL().RawQuery, headers, a.manager.RouterBaseURL(), upstreamBody)
	if err != nil {
		a.log.Warn("proxy failed", "model", req.Model, "error", err)
		a.writeOpenAIError(ctx, http.StatusServiceUnavailable, "upstream_error", "router_unavailable", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	a.writeUpstreamResponse(ctx, resp, req.Model)
}

func (a *App) writeEnsureError(ctx huma.Context, model string, err error) {
	switch {
	case errors.Is(err, routermanager.ErrModelNotFound):
		a.writeOpenAIError(ctx, http.StatusNotFound, "invalid_request_error", "model_not_found", "model is not in catalog")
	case errors.Is(err, routermanager.ErrCapabilityMismatch):
		a.writeOpenAIError(ctx, http.StatusBadRequest, "invalid_request_error", "model_capability_mismatch", err.Error())
	case errors.Is(err, routermanager.ErrDownloadFailed):
		a.log.Error("download failed", "model", model, "error", err)
		a.writeOpenAIError(ctx, http.StatusServiceUnavailable, "download_error", "download_failed", err.Error())
	case errors.Is(err, routermanager.ErrRouterReloadFailed):
		a.log.Error("router reload failed", "model", model, "error", err)
		a.writeOpenAIError(ctx, http.StatusServiceUnavailable, "upstream_error", "router_reload_failed", err.Error())
	default:
		a.log.Error("ensure available failed", "model", model, "error", err)
		a.writeOpenAIError(ctx, http.StatusServiceUnavailable, "upstream_error", "ensure_available_failed", err.Error())
	}
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("request body too large")
	}
	return body, nil
}

func appendResponseHeaders(ctx huma.Context, headers http.Header) {
	for k, vals := range headers {
		if proxy.IsHopHeader(k) {
			continue
		}
		for _, v := range vals {
			ctx.AppendHeader(k, v)
		}
	}
}

func (a *App) writeJSON(ctx huma.Context, status int, v any) {
	ctx.SetHeader("Content-Type", "application/json; charset=utf-8")
	ctx.SetStatus(status)
	if err := json.NewEncoder(ctx.BodyWriter()).Encode(v); err != nil {
		a.log.Debug("write JSON response failed", "path", ctx.URL().Path, "error", err)
	}
}

func (a *App) writeOpenAIError(ctx huma.Context, status int, typ, code, msg string) {
	a.writeJSON(ctx, status, openaiapi.ErrorBody{Error: openaiapi.ErrorObject{Message: msg, Type: typ, Code: stringPtr(code)}})
}

func requiredKind(path string) string {
	if path == "/v1/embeddings" {
		return "embedding"
	}
	return "chat"
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
