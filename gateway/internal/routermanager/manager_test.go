package routermanager

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/catalog"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/hf"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/routerclient"
)

func TestEnsureAvailableDownloadsRendersAndReloads(t *testing.T) {
	t.Parallel()

	modelsDir := t.TempDir()
	model := catalog.Model{Repo: "owner/repo", Quant: "Q4_K_M", File: "model-Q4_K_M.gguf"}
	cat := &catalog.Catalog{Models: []catalog.Model{model}}

	hfServer := newTestHFServer(t, model.File)
	var reloads atomic.Int32
	routerServer := newTestRouterServer(t, &reloads, model.Ref(), false, true)

	mgr := New(slog.New(slog.NewTextHandler(os.Stderr, nil)), cat, &hf.Downloader{Endpoint: hfServer.URL, ModelsDir: modelsDir}, routerclient.New(routerServer.URL), Config{ModelsDir: modelsDir, PresetPath: filepath.Join(modelsDir, "models.ini"), ReloadTimeout: time.Second})
	if err := mgr.RenderPreset(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.EnsureAvailable(context.Background(), model.Ref(), "chat"); err != nil {
		t.Fatal(err)
	}
	if reloads.Load() != 1 {
		t.Fatalf("reloads=%d, want 1", reloads.Load())
	}
	if _, err := os.Stat(model.StablePath(modelsDir)); err != nil {
		t.Fatalf("downloaded model missing: %v", err)
	}
}

func TestEnsureAvailableSkipsReloadWhenAlreadyRegistered(t *testing.T) {
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

	var reloads atomic.Int32
	routerServer := newTestRouterServer(t, &reloads, model.Ref(), true, true)
	mgr := New(slog.New(slog.NewTextHandler(os.Stderr, nil)), cat, &hf.Downloader{Endpoint: "http://127.0.0.1:1", ModelsDir: modelsDir}, routerclient.New(routerServer.URL), Config{ModelsDir: modelsDir, PresetPath: filepath.Join(modelsDir, "models.ini"), ReloadTimeout: time.Second})
	if err := mgr.RenderPreset(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.EnsureAvailable(context.Background(), model.Ref(), "chat"); err != nil {
		t.Fatal(err)
	}
	if reloads.Load() != 0 {
		t.Fatalf("reloads=%d, want 0", reloads.Load())
	}
}

func TestEnsureAvailableReloadsWhenPresetRenderedButRouterRegistryStale(t *testing.T) {
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

	var reloads atomic.Int32
	routerServer := newTestRouterServer(t, &reloads, model.Ref(), false, true)
	mgr := New(slog.New(slog.NewTextHandler(os.Stderr, nil)), cat, &hf.Downloader{Endpoint: "http://127.0.0.1:1", ModelsDir: modelsDir}, routerclient.New(routerServer.URL), Config{ModelsDir: modelsDir, PresetPath: filepath.Join(modelsDir, "models.ini"), ReloadTimeout: time.Second})
	if err := mgr.RenderPreset(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.EnsureAvailable(context.Background(), model.Ref(), "chat"); err != nil {
		t.Fatal(err)
	}
	if reloads.Load() != 1 {
		t.Fatalf("reloads=%d, want 1", reloads.Load())
	}
}

func TestEnsureAvailableFailsWhenReloadDoesNotRegisterModel(t *testing.T) {
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

	var reloads atomic.Int32
	routerServer := newTestRouterServer(t, &reloads, model.Ref(), false, false)
	mgr := New(slog.New(slog.NewTextHandler(os.Stderr, nil)), cat, &hf.Downloader{Endpoint: "http://127.0.0.1:1", ModelsDir: modelsDir}, routerclient.New(routerServer.URL), Config{ModelsDir: modelsDir, PresetPath: filepath.Join(modelsDir, "models.ini"), ReloadTimeout: time.Second})
	if err := mgr.RenderPreset(); err != nil {
		t.Fatal(err)
	}
	err := mgr.EnsureAvailable(context.Background(), model.Ref(), "chat")
	if !errors.Is(err, ErrRouterReloadFailed) {
		t.Fatalf("err=%v, want router reload failure", err)
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

func newTestRouterServer(t *testing.T, reloads *atomic.Int32, modelRef string, registeredInitially, registeredAfterReload bool) *httptest.Server {
	t.Helper()
	var reloaded atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			if r.URL.Query().Get("reload") == "1" {
				reloads.Add(1)
				reloaded.Store(true)
			}
			registered := registeredInitially || (reloaded.Load() && registeredAfterReload)
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
