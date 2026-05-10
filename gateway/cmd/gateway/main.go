package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/catalog"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/config"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/hf"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/proxy"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/routerclient"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/routermanager"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/server"
)

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
	mgr := routermanager.New(log, cat, dl, routerclient.New(routerURL), routermanager.Config{ModelsDir: modelsDir, PresetPath: config.String("LLAMA_MODELS_PRESET", modelsDir+"/models-preset.generated.ini"), CtxSize: config.Int("LLAMA_ROUTER_CTX_SIZE", 8192), Parallel: config.Int("LLAMA_ROUTER_PARALLEL", -1), ThreadsHTTP: config.Int("LLAMA_ROUTER_THREADS_HTTP", -1), NGPULayers: config.Int("LLAMA_ROUTER_N_GPU_LAYERS", 999), ExtraArgs: config.String("LLAMA_ROUTER_EXTRA_ARGS", ""), ReloadTimeout: config.DurationSeconds("LLAMA_ROUTER_RELOAD_TIMEOUT_SECONDS", 30*time.Second)})
	if err := mgr.RenderPreset(); err != nil {
		log.Error("render initial preset", "error", err)
		os.Exit(1)
	}

	addr := config.String("GATEWAY_ADDR", ":8090")
	srv := server.NewHTTPServer(addr, server.New(log, mgr, proxy.Proxy{}).Handler())
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
