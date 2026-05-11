package routermanager

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/catalog"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/hf"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/routerclient"
)

func TestEnsureAvailableDownloadsAndRequiresRouterRegistration(t *testing.T) {
	t.Parallel()

	modelsDir := t.TempDir()
	model := catalog.Model{Repo: "owner/repo", Quant: "Q4_K_M", File: "model-Q4_K_M.gguf"}
	cat := &catalog.Catalog{Models: []catalog.Model{model}}

	hfServer := newTestHFServer(t, model.File)
	routerServer := newTestRouterServer(t, model.Ref(), true)

	mgr := New(slog.New(slog.NewTextHandler(os.Stderr, nil)), cat, &hf.Downloader{Endpoint: hfServer.URL, ModelsDir: modelsDir}, routerclient.New(routerServer.URL), Config{ModelsDir: modelsDir, PresetPath: filepath.Join(modelsDir, "models.ini")})
	if err := mgr.RenderPreset(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.EnsureAvailable(context.Background(), model.Ref(), "chat"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(model.StablePath(modelsDir)); err != nil {
		t.Fatalf("downloaded model missing: %v", err)
	}
}

func TestEnsureAvailableFailsWhenRouterRegistryMissingModel(t *testing.T) {
	t.Parallel()

	modelsDir := t.TempDir()
	model := catalog.Model{Repo: "owner/repo", Quant: "Q4_K_M", File: "model-Q4_K_M.gguf"}
	cat := &catalog.Catalog{Models: []catalog.Model{model}}
	if err := os.MkdirAll(filepath.Dir(model.StablePath(modelsDir)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(model.StablePath(modelsDir), []byte("already downloaded"), 0o644); err != nil {
		t.Fatal(err)
	}

	routerServer := newTestRouterServer(t, model.Ref(), false)
	mgr := New(slog.New(slog.NewTextHandler(os.Stderr, nil)), cat, &hf.Downloader{Endpoint: "http://127.0.0.1:1", ModelsDir: modelsDir}, routerclient.New(routerServer.URL), Config{ModelsDir: modelsDir, PresetPath: filepath.Join(modelsDir, "models.ini")})
	if err := mgr.RenderPreset(); err != nil {
		t.Fatal(err)
	}
	err := mgr.EnsureAvailable(context.Background(), model.Ref(), "chat")
	if !errors.Is(err, ErrRouterRegistryStale) {
		t.Fatalf("err=%v, want router registry stale", err)
	}
}

func TestEnsureAvailableRejectsCapabilityMismatch(t *testing.T) {
	t.Parallel()

	model := catalog.Model{Repo: "owner/repo", Quant: "Q4_K_M", Kind: "embedding"}
	mgr := New(slog.Default(), &catalog.Catalog{Models: []catalog.Model{model}}, &hf.Downloader{}, routerclient.Client{}, Config{})
	err := mgr.EnsureAvailable(context.Background(), model.Ref(), "chat")
	if !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("err=%v, want capability mismatch", err)
	}
}

func newTestHFServer(t *testing.T, file string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models/owner/repo/tree/main":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"type":"file","path":"` + file + `","size":123}]`))
		case "/owner/repo/resolve/main/" + file:
			_, _ = w.Write([]byte("gguf bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestRouterServer(t *testing.T, modelRef string, registered bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Header().Set("Content-Type", "application/json")
			if registered {
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"` + modelRef + `","aliases":["` + modelRef + `"],"object":"model","status":{"value":"unloaded"}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		case "/health":
			_, _ = w.Write([]byte(`OK`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
