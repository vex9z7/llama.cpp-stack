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

	"github.com/vex9z7/llama.cpp-stack/internal/catalog"
	"github.com/vex9z7/llama.cpp-stack/internal/config"
	"github.com/vex9z7/llama.cpp-stack/internal/hf"
	"github.com/vex9z7/llama.cpp-stack/internal/manager"
	"github.com/vex9z7/llama.cpp-stack/internal/openai"
	"github.com/vex9z7/llama.cpp-stack/internal/proxy"
	"github.com/vex9z7/llama.cpp-stack/internal/workerclient"
)

type app struct {
	log     *slog.Logger
	manager *manager.Manager
	proxy   proxy.Proxy
}

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
	workerURLs := config.CSV("WORKER_BASE_URLS")
	workers := make([]workerclient.Client, 0, len(workerURLs))
	for _, u := range workerURLs {
		workers = append(workers, workerclient.New(u))
	}
	if len(workers) == 0 {
		log.Warn("WORKER_BASE_URLS is empty; gateway can list models but cannot serve inference")
	}
	dl := &hf.Downloader{Endpoint: config.String("HF_ENDPOINT", "https://huggingface.co"), Token: config.String("HF_TOKEN", ""), ModelsDir: modelsDir}
	mgr := manager.New(log, cat, dl, workers, manager.Config{ModelsDir: modelsDir, CtxSize: config.Int("LLAMA_WORKER_CTX_SIZE", 8192), Parallel: config.Int("LLAMA_WORKER_PARALLEL", 1), ThreadsHTTP: config.Int("LLAMA_WORKER_THREADS_HTTP", -1), NGPULayers: config.Int("LLAMA_WORKER_N_GPU_LAYERS", 999), ExtraArgs: config.String("LLAMA_WORKER_EXTRA_ARGS", ""), StartTimeout: config.DurationSeconds("LLAMA_INSTANCE_START_TIMEOUT_SECONDS", 120*time.Second)})
	mgr.Reconcile(context.Background())
	a := &app{log: log, manager: mgr, proxy: proxy.Proxy{}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /v1/models", a.models)
	mux.HandleFunc("POST /v1/chat/completions", a.inference)
	mux.HandleFunc("POST /v1/completions", a.inference)
	mux.HandleFunc("POST /v1/responses", a.inference)
	mux.HandleFunc("POST /v1/embeddings", a.inference)

	addr := config.String("GATEWAY_ADDR", ":8090")
	srv := &http.Server{Addr: addr, Handler: logMiddleware(log, mux), ReadHeaderTimeout: 15 * time.Second}
	go func() {
		log.Info("gateway listening", "addr", addr)
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

func (a *app) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "llama.cpp-stack-gateway"})
}

func (a *app) models(w http.ResponseWriter, r *http.Request) {
	models := a.manager.ListModels(r.Context())
	data := make([]map[string]any, 0, len(models))
	for _, m := range models {
		data = append(data, map[string]any{"id": m.ID, "object": "model", "owned_by": m.OwnedBy, "meta": map[string]any{"downloaded": m.Downloaded, "running": m.Running, "cold_start": m.ColdStart, "repo": m.Repo, "quant": m.Quant, "kind": m.Kind}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (a *app) inference(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_body", "failed to read request body")
		return
	}
	defer r.Body.Close()
	var req modelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "request body must be JSON")
		return
	}
	if req.Model == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "missing_model", "request body must include model")
		return
	}
	backend, err := a.manager.EnsureRunning(r.Context(), req.Model)
	if err != nil {
		switch {
		case errors.Is(err, manager.ErrModelNotFound):
			openai.WriteError(w, http.StatusNotFound, "invalid_request_error", "model_not_found", "model is not in catalog")
		case errors.Is(err, manager.ErrNoIdleWorker):
			openai.WriteError(w, http.StatusTooManyRequests, "capacity_error", "no_idle_worker", "requested model is not loaded and no idle worker is available")
		default:
			a.log.Error("ensure running failed", "model", req.Model, "error", err)
			openai.WriteError(w, http.StatusServiceUnavailable, "startup_error", "worker_load_failed", err.Error())
		}
		return
	}
	if err := a.proxy.ForwardBytes(r.Context(), w, r, backend.InferenceURL, body); err != nil {
		a.log.Warn("proxy failed", "model", req.Model, "worker", backend.WorkerID, "error", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info("request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}
