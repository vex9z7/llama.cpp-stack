package routermanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/catalog"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/hf"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/preset"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/routerclient"
)

var (
	ErrModelNotFound      = errors.New("model not found in catalog")
	ErrCapabilityMismatch = errors.New("model capability mismatch")
	ErrDownloadFailed     = errors.New("model download failed")
	ErrRouterReloadFailed = errors.New("router reload failed")
	ErrRouterUnavailable  = errors.New("router unavailable")
)

type Config struct {
	ModelsDir     string
	PresetPath    string
	CtxSize       int
	Parallel      int
	ThreadsHTTP   int
	NGPULayers    int
	ExtraArgs     string
	ReloadTimeout time.Duration
}

type ModelStatus struct {
	ID           string          `json:"id"`
	Object       string          `json:"object"`
	OwnedBy      string          `json:"owned_by"`
	Downloaded   bool            `json:"downloaded"`
	RouterStatus string          `json:"router_status,omitempty"`
	Running      bool            `json:"running"`
	ColdStart    bool            `json:"cold_start"`
	Repo         string          `json:"repo"`
	Quant        string          `json:"quant"`
	Kind         string          `json:"kind,omitempty"`
	RouterMeta   json.RawMessage `json:"router_meta,omitempty"`
}

type Manager struct {
	log        *slog.Logger
	catalog    *catalog.Catalog
	downloader *hf.Downloader
	router     routerclient.Client
	cfg        Config

	mu         sync.Mutex
	modelLocks map[string]*sync.Mutex
	lastRender preset.Rendered
}

func New(log *slog.Logger, cat *catalog.Catalog, dl *hf.Downloader, router routerclient.Client, cfg Config) *Manager {
	return &Manager{log: log, catalog: cat, downloader: dl, router: router, cfg: cfg, modelLocks: map[string]*sync.Mutex{}}
}

func KindOf(cm catalog.Model) string {
	if cm.Kind == "" {
		return "chat"
	}
	return cm.Kind
}

func (m *Manager) RouterBaseURL() string { return m.router.BaseURL }

func (m *Manager) RenderPreset() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := preset.Render(m.catalog, preset.Config{ModelsDir: m.cfg.ModelsDir, Path: m.cfg.PresetPath, CtxSize: m.cfg.CtxSize, Parallel: m.cfg.Parallel, ThreadsHTTP: m.cfg.ThreadsHTTP, NGPULayers: m.cfg.NGPULayers, ExtraArgs: m.cfg.ExtraArgs})
	if err != nil {
		return err
	}
	m.lastRender = r
	m.log.Info("rendered router preset", "path", r.Path, "models", r.IncludedCount)
	return nil
}

func (m *Manager) Health(ctx context.Context) error {
	if err := m.router.Health(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrRouterUnavailable, err)
	}
	return nil
}

func (m *Manager) ListModels(ctx context.Context) []ModelStatus {
	routerStatuses := map[string]routerclient.ModelRecord{}
	if resp, err := m.router.Models(ctx, false); err == nil {
		for _, rec := range resp.Data {
			routerStatuses[rec.ID] = rec
		}
	} else {
		m.log.Debug("router models unavailable", "error", err)
	}
	out := make([]ModelStatus, 0, len(m.catalog.Models))
	for _, cm := range m.catalog.Models {
		rec, hasRouter := routerStatuses[cm.Ref()]
		status := parseStatusValue(rec.Status)
		down := fileExists(cm.StablePath(m.cfg.ModelsDir))
		running := status == "loaded" || status == "loading" || status == "sleeping"
		if !hasRouter && down {
			status = "downloaded"
		}
		out = append(out, ModelStatus{ID: cm.Ref(), Object: "model", OwnedBy: "llama.cpp-stack", Downloaded: down, RouterStatus: status, Running: running, ColdStart: !running, Repo: cm.Repo, Quant: cm.Quant, Kind: KindOf(cm), RouterMeta: rec.Meta})
	}
	return out
}

func (m *Manager) EnsureAvailable(ctx context.Context, ref, requiredKind string) error {
	cm, ok := m.catalog.ByRef(ref)
	if !ok {
		return ErrModelNotFound
	}
	kind := KindOf(cm)
	if requiredKind != "" && kind != requiredKind {
		return fmt.Errorf("%w: model %s has kind %s but endpoint requires %s", ErrCapabilityMismatch, cm.Ref(), kind, requiredKind)
	}
	lock := m.lockFor(cm.Ref())
	lock.Lock()
	defer lock.Unlock()

	stablePath := cm.StablePath(m.cfg.ModelsDir)
	alreadyDownloaded := fileExists(stablePath)
	alreadyRendered := m.hasRenderedModel(cm.Ref())
	if _, err := m.downloader.Ensure(ctx, cm); err != nil {
		return fmt.Errorf("%w: code=%s: %w", ErrDownloadFailed, hf.Code(err), err)
	}
	if alreadyDownloaded && alreadyRendered {
		m.log.Debug("model already available", "model", cm.Ref(), "path", stablePath)
		return nil
	}
	if err := m.RenderPreset(); err != nil {
		return fmt.Errorf("%w: render preset: %w", ErrRouterReloadFailed, err)
	}
	reloadCtx := ctx
	if m.cfg.ReloadTimeout > 0 {
		var cancel context.CancelFunc
		reloadCtx, cancel = context.WithTimeout(ctx, m.cfg.ReloadTimeout)
		defer cancel()
	}
	if _, err := m.router.Models(reloadCtx, true); err != nil {
		return fmt.Errorf("%w: %w", ErrRouterReloadFailed, err)
	}
	return nil
}

func (m *Manager) hasRenderedModel(ref string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, included := range m.lastRender.IncludedRefs {
		if included == ref {
			return true
		}
	}
	return false
}

func (m *Manager) lockFor(ref string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.modelLocks[ref]
	if l == nil {
		l = &sync.Mutex{}
		m.modelLocks[ref] = l
	}
	return l
}

func parseStatusValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var obj struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	return obj.Value
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Size() > 0
}
