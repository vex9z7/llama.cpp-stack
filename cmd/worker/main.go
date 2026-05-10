package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vex9z7/llama.cpp-stack/internal/config"
	"github.com/vex9z7/llama.cpp-stack/internal/llamaprocess"
	"github.com/vex9z7/llama.cpp-stack/internal/openai"
)

type app struct {
	log *slog.Logger
	sup *llamaprocess.Supervisor
}

type unloadRequest struct {
	Force bool `json:"force"`
}

func main() {
	log := config.Logger()
	slog.SetDefault(log)
	workerID := config.String("WORKER_ID", "worker-0")
	serverAddr := config.String("LLAMA_SERVER_ADDR", ":8080")
	_, serverPort := splitAddr(serverAddr)
	sup := llamaprocess.New(workerID, config.String("MODELS_DIR", "/models"), config.String("LLAMA_SERVER_BIN", ""), "0.0.0.0", serverPort, config.DurationSeconds("LLAMA_INSTANCE_START_TIMEOUT_SECONDS", 120*time.Second))
	a := &app{log: log, sup: sup}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /worker/health", a.health)
	mux.HandleFunc("GET /worker/status", a.status)
	mux.HandleFunc("POST /worker/load", a.load)
	mux.HandleFunc("POST /worker/unload", a.unload)

	addr := config.String("WORKER_AGENT_ADDR", ":8092")
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 15 * time.Second}
	go func() {
		log.Info("worker listening", "worker", workerID, "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("worker failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	unloadCtx, unloadCancel := context.WithTimeout(context.Background(), 20*time.Second)
	_ = sup.Unload(unloadCtx, true)
	unloadCancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	_ = srv.Shutdown(shutdownCtx)
	shutdownCancel()
}

func (a *app) health(w http.ResponseWriter, r *http.Request) {
	st := a.sup.Status()
	writeJSON(w, map[string]any{"status": "ok", "state": st.State})
}

func (a *app) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.sup.Status())
}

func (a *app) load(w http.ResponseWriter, r *http.Request) {
	var req llamaprocess.LoadRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "request body must be JSON")
		return
	}
	st, err := a.sup.Load(r.Context(), req)
	if err != nil {
		if err.Error() == "worker already has a loaded model" {
			openai.WriteError(w, http.StatusConflict, "worker_busy", "worker_busy", err.Error())
			return
		}
		a.log.Error("load failed", "error", err)
		openai.WriteError(w, http.StatusServiceUnavailable, "startup_error", "worker_load_failed", err.Error())
		return
	}
	writeJSON(w, map[string]any{"status": "loaded", "worker": st.ID, "inference_url": st.InferenceURL})
}

func (a *app) unload(w http.ResponseWriter, r *http.Request) {
	var req unloadRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	if err := a.sup.Unload(r.Context(), req.Force); err != nil {
		openai.WriteError(w, http.StatusConflict, "worker_busy", "active_requests", err.Error())
		return
	}
	writeJSON(w, map[string]any{"status": "unloaded", "worker": a.sup.Status().ID})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

func splitAddr(addr string) (string, string) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:]
		}
	}
	return "", addr
}
