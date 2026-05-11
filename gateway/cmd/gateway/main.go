package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

	if err := run(log); err != nil {
		log.Error("gateway exited", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	if len(os.Args) > 1 && os.Args[1] == "render-preset" {
		return renderPresetOnly(log)
	}

	modelsDir := config.String("MODELS_DIR", "/models")
	catalogPath := config.String("CATALOG_PATH", "/configs/models.catalog.toml")
	cat, err := catalog.Load(catalogPath)
	if err != nil {
		return fmt.Errorf("load catalog %q: %w", catalogPath, err)
	}

	routerURL := config.String("LLAMA_ROUTER_URL", "http://llama-router:8080")
	token, err := hfToken()
	if err != nil {
		return err
	}
	dl := &hf.Downloader{
		Endpoint:  config.String("HF_ENDPOINT", "https://huggingface.co"),
		Token:     token,
		ModelsDir: modelsDir,
	}
	routerCfg := routermanager.Config{
		ModelsDir:   modelsDir,
		PresetPath:  config.String("LLAMA_MODELS_PRESET", modelsDir+"/models-preset.generated.ini"),
		CtxSize:     config.Int("LLAMA_ROUTER_CTX_SIZE", 8192),
		Parallel:    config.Int("LLAMA_ROUTER_PARALLEL", -1),
		ThreadsHTTP: config.Int("LLAMA_ROUTER_THREADS_HTTP", -1),
		NGPULayers:  config.Int("LLAMA_ROUTER_N_GPU_LAYERS", 999),
		ExtraArgs:   config.String("LLAMA_ROUTER_EXTRA_ARGS", ""),
	}
	mgr := routermanager.New(log, cat, dl, routerclient.New(routerURL), routerCfg)
	if err := mgr.RenderPreset(); err != nil {
		return fmt.Errorf("render initial preset: %w", err)
	}
	addr := config.String("GATEWAY_ADDR", ":8090")
	srv := server.NewHTTPServer(addr, server.New(log, mgr, proxy.Proxy{}).Handler())
	serverErr := make(chan error, 1)
	go func() {
		log.Info("gateway listening", "addr", addr, "router_url", routerURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case sig := <-stop:
		log.Info("gateway shutting down", "signal", sig.String())
	case err := <-serverErr:
		if err != nil {
			return err
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	return <-serverErr
}

func renderPresetOnly(log *slog.Logger) error {
	modelsDir := config.String("MODELS_DIR", "/models")
	catalogPath := config.String("CATALOG_PATH", "/configs/models.catalog.toml")
	cat, err := catalog.Load(catalogPath)
	if err != nil {
		return fmt.Errorf("load catalog %q: %w", catalogPath, err)
	}
	cfg := routermanager.Config{
		ModelsDir:   modelsDir,
		PresetPath:  config.String("LLAMA_MODELS_PRESET", modelsDir+"/models-preset.generated.ini"),
		CtxSize:     config.Int("LLAMA_ROUTER_CTX_SIZE", 8192),
		Parallel:    config.Int("LLAMA_ROUTER_PARALLEL", -1),
		ThreadsHTTP: config.Int("LLAMA_ROUTER_THREADS_HTTP", -1),
		NGPULayers:  config.Int("LLAMA_ROUTER_N_GPU_LAYERS", 999),
		ExtraArgs:   config.String("LLAMA_ROUTER_EXTRA_ARGS", ""),
	}
	mgr := routermanager.New(log, cat, &hf.Downloader{ModelsDir: modelsDir}, routerclient.Client{}, cfg)
	return mgr.RenderPreset()
}

func hfToken() (string, error) {
	tokenFile := config.String("HF_TOKEN_FILE", "")
	if tokenFile == "" {
		return config.String("HF_TOKEN", ""), nil
	}
	b, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", fmt.Errorf("read HF_TOKEN_FILE %q: %w", tokenFile, err)
	}
	return strings.TrimSpace(string(b)), nil
}
