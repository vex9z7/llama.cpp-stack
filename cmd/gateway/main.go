package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/vex9z7/llama.cpp-stack/internal/catalog"
	"github.com/vex9z7/llama.cpp-stack/internal/config"
	"github.com/vex9z7/llama.cpp-stack/internal/hf"
	"github.com/vex9z7/llama.cpp-stack/internal/proxy"
	"github.com/vex9z7/llama.cpp-stack/internal/routerclient"
	"github.com/vex9z7/llama.cpp-stack/internal/routermanager"
)

type app struct {
	log     *slog.Logger
	manager *routermanager.Manager
	proxy   proxy.Proxy
}

const maxInferenceBodyBytes int64 = 32 << 20

type modelRequest struct {
	Model string `json:"model"`
}

func main() {
	log := config.Logger()
	slog.SetDefault(log)
	modelsDir := config.String("MODELS_DIR", "/models")
	catalogPath := config.String("CATALOG_PATH", modelsDir+"/catalog.toml")
	cat, err := catalog.Load(catalogPath)
	if err != nil {
		log.Error("load catalog", "path", catalogPath, "error", err)
		os.Exit(1)
	}
	routerURL := config.String("LLAMA_ROUTER_URL", "http://llama-router:8080")
	dl := &hf.Downloader{Endpoint: config.String("HF_ENDPOINT", "https://huggingface.co"), Token: config.String("HF_TOKEN", ""), ModelsDir: modelsDir}
	mgr := routermanager.New(log, cat, dl, routerclient.New(routerURL), routermanager.Config{ModelsDir: modelsDir, PresetPath: config.String("LLAMA_MODELS_PRESET", modelsDir+"/models-preset.generated.ini"), CtxSize: config.Int("LLAMA_ROUTER_CTX_SIZE", config.Int("LLAMA_WORKER_CTX_SIZE", 8192)), Parallel: config.Int("LLAMA_ROUTER_PARALLEL", config.Int("LLAMA_WORKER_PARALLEL", 1)), ThreadsHTTP: config.Int("LLAMA_ROUTER_THREADS_HTTP", config.Int("LLAMA_WORKER_THREADS_HTTP", -1)), NGPULayers: config.Int("LLAMA_ROUTER_N_GPU_LAYERS", config.Int("LLAMA_WORKER_N_GPU_LAYERS", 999)), ExtraArgs: config.String("LLAMA_ROUTER_EXTRA_ARGS", config.String("LLAMA_WORKER_EXTRA_ARGS", "")), ReloadTimeout: config.DurationSeconds("LLAMA_ROUTER_RELOAD_TIMEOUT_SECONDS", 30*time.Second)})
	if err := mgr.RenderPreset(); err != nil {
		log.Error("render initial preset", "error", err)
		os.Exit(1)
	}
	a := &app{log: log, manager: mgr, proxy: proxy.Proxy{}}

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("llama.cpp-stack Gateway", "0.2.0"))
	a.register(api)

	addr := config.String("GATEWAY_ADDR", ":8090")
	srv := &http.Server{Addr: addr, Handler: logMiddleware(log, r), ReadHeaderTimeout: 15 * time.Second}
	go func() {
		log.Info("gateway listening", "addr", addr, "router_url", routerURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("gateway failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func (a *app) register(api huma.API) {
	registerRaw(api, withJSONResponse(&huma.Operation{OperationID: "getHealth", Method: http.MethodGet, Path: "/health", Summary: "Gateway health", Tags: []string{"system"}}), a.humaHealth)
	registerRaw(api, withJSONResponse(&huma.Operation{OperationID: "listModels", Method: http.MethodGet, Path: "/v1/models", Summary: "List catalog models", Tags: []string{"models"}}), a.humaModels)
	for _, path := range []string{"/v1/chat/completions", "/v1/completions", "/v1/responses", "/v1/embeddings"} {
		registerRaw(api, withProxyDocs(&huma.Operation{OperationID: operationID(path), Method: http.MethodPost, Path: path, Summary: "Proxy OpenAI-compatible inference request", Tags: []string{"inference"}, MaxBodyBytes: maxInferenceBodyBytes, SkipValidateBody: true, Errors: []int{400, 404, 429, 503}}), a.humaInference)
	}
}

func registerRaw(api huma.API, op *huma.Operation, handler func(huma.Context)) {
	api.OpenAPI().AddOperation(op)
	api.Adapter().Handle(op, handler)
}

func withJSONResponse(op *huma.Operation) *huma.Operation {
	op.Responses = map[string]*huma.Response{"200": jsonResponse("JSON response")}
	return op
}

func withProxyDocs(op *huma.Operation) *huma.Operation {
	op.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{Type: "object", Required: []string{"model"}, AdditionalProperties: true, Properties: map[string]*huma.Schema{"model": {Type: "string"}}}}}}
	op.Responses = map[string]*huma.Response{
		"200": jsonResponse("Successful upstream response. Shape depends on the proxied llama.cpp/OpenAI-compatible endpoint."),
		"400": jsonResponse("Gateway validation error"),
		"404": jsonResponse("Catalog model not found"),
		"429": jsonResponse("Strict capacity error, if enabled"),
		"503": jsonResponse("Download, router reload, or upstream availability error"),
	}
	return op
}

func jsonResponse(desc string) *huma.Response {
	return &huma.Response{Description: desc, Content: map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{Type: "object", AdditionalProperties: true}}}}
}

func (a *app) humaHealth(ctx huma.Context) {
	status := "ok"
	router := "ok"
	if err := a.manager.Health(ctx.Context()); err != nil {
		status = "degraded"
		router = "unavailable"
	}
	writeHumaJSON(ctx, http.StatusOK, map[string]any{"status": status, "service": "llama.cpp-stack-gateway", "router": router})
}

func (a *app) humaModels(ctx huma.Context) {
	models := a.manager.ListModels(ctx.Context())
	data := make([]map[string]any, 0, len(models))
	for _, m := range models {
		data = append(data, map[string]any{"id": m.ID, "object": "model", "owned_by": m.OwnedBy, "meta": map[string]any{"downloaded": m.Downloaded, "router_status": m.RouterStatus, "running": m.Running, "cold_start": m.ColdStart, "repo": m.Repo, "quant": m.Quant, "kind": m.Kind}})
	}
	writeHumaJSON(ctx, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (a *app) humaInference(ctx huma.Context) {
	body, err := readLimited(ctx.BodyReader(), maxInferenceBodyBytes)
	if err != nil {
		writeOpenAIErrorHuma(ctx, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return
	}
	var req modelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeOpenAIErrorHuma(ctx, http.StatusBadRequest, "invalid_request_error", "invalid_json", "request body must be JSON")
		return
	}
	if req.Model == "" {
		writeOpenAIErrorHuma(ctx, http.StatusBadRequest, "invalid_request_error", "missing_model", "request body must include model")
		return
	}
	if err := a.manager.EnsureAvailable(ctx.Context(), req.Model, requiredKind(ctx.URL().Path)); err != nil {
		a.writeEnsureError(ctx, req.Model, err)
		return
	}
	headers := http.Header{}
	ctx.EachHeader(func(name, value string) { headers.Add(name, value) })
	resp, err := a.proxy.Do(ctx.Context(), ctx.Method(), ctx.URL().Path, ctx.URL().RawQuery, headers, a.manager.RouterBaseURL(), body)
	if err != nil {
		a.log.Warn("proxy failed", "model", req.Model, "error", err)
		writeOpenAIErrorHuma(ctx, http.StatusServiceUnavailable, "upstream_error", "router_unavailable", err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	appendResponseHeaders(ctx, resp.Header)
	if resp.Header.Get("Content-Type") == "" {
		ctx.SetHeader("Content-Type", "application/json; charset=utf-8")
	}
	ctx.SetStatus(resp.StatusCode)
	if err := proxy.CopyFlush(ctx.BodyWriter(), resp.Body); err != nil {
		a.log.Warn("proxy copy failed", "model", req.Model, "error", err)
	}
}

func (a *app) writeEnsureError(ctx huma.Context, model string, err error) {
	switch {
	case errors.Is(err, routermanager.ErrModelNotFound):
		writeOpenAIErrorHuma(ctx, http.StatusNotFound, "invalid_request_error", "model_not_found", "model is not in catalog")
	case errors.Is(err, routermanager.ErrCapabilityMismatch):
		writeOpenAIErrorHuma(ctx, http.StatusBadRequest, "invalid_request_error", "model_capability_mismatch", err.Error())
	case errors.Is(err, routermanager.ErrDownloadFailed):
		a.log.Error("download failed", "model", model, "error", err)
		writeOpenAIErrorHuma(ctx, http.StatusServiceUnavailable, "download_error", "download_failed", err.Error())
	case errors.Is(err, routermanager.ErrRouterReloadFailed):
		a.log.Error("router reload failed", "model", model, "error", err)
		writeOpenAIErrorHuma(ctx, http.StatusServiceUnavailable, "upstream_error", "router_reload_failed", err.Error())
	default:
		a.log.Error("ensure available failed", "model", model, "error", err)
		writeOpenAIErrorHuma(ctx, http.StatusServiceUnavailable, "upstream_error", "ensure_available_failed", err.Error())
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

func writeHumaJSON(ctx huma.Context, status int, v any) {
	ctx.SetHeader("Content-Type", "application/json; charset=utf-8")
	ctx.SetStatus(status)
	_ = json.NewEncoder(ctx.BodyWriter()).Encode(v)
}

func writeOpenAIErrorHuma(ctx huma.Context, status int, typ, code, msg string) {
	ctx.SetHeader("Content-Type", "application/json; charset=utf-8")
	ctx.SetStatus(status)
	_ = json.NewEncoder(ctx.BodyWriter()).Encode(map[string]any{"error": map[string]any{"message": msg, "type": typ, "code": code}})
}

func logMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info("request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}

func requiredKind(path string) string {
	if path == "/v1/embeddings" {
		return "embedding"
	}
	return "chat"
}

func operationID(path string) string {
	switch path {
	case "/v1/chat/completions":
		return "createChatCompletion"
	case "/v1/completions":
		return "createCompletion"
	case "/v1/responses":
		return "createResponse"
	case "/v1/embeddings":
		return "createEmbedding"
	default:
		return "proxyInference"
	}
}
