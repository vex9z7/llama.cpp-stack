package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/proxy"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/routermanager"
)

type App struct {
	log     *slog.Logger
	manager *routermanager.Manager
	proxy   proxy.Proxy
}

func New(log *slog.Logger, manager *routermanager.Manager, proxy proxy.Proxy) *App {
	return &App{log: log, manager: manager, proxy: proxy}
}

func (a *App) Handler() http.Handler {
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("llama.cpp-stack Gateway", "0.2.0"))
	a.Register(api)
	return logMiddleware(a.log, r)
}

func NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 15 * time.Second}
}
