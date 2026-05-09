package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vex9z7/llama.cpp-stack/internal/catalog"
	"github.com/vex9z7/llama.cpp-stack/internal/hf"
	"github.com/vex9z7/llama.cpp-stack/internal/workerclient"
)

var (
	ErrModelNotFound = errors.New("model not found in catalog")
	ErrNoIdleWorker  = errors.New("no idle worker")
)

type Config struct {
	ModelsDir    string
	CtxSize      int
	Parallel     int
	ThreadsHTTP  int
	NGPULayers   int
	ExtraArgs    string
	StartTimeout time.Duration
}

type RunningBackend struct {
	ModelRef     string
	WorkerID     string
	InferenceURL string
}

type ModelStatus struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	OwnedBy    string `json:"owned_by"`
	Downloaded bool   `json:"downloaded"`
	Running    bool   `json:"running"`
	ColdStart  bool   `json:"cold_start"`
	Repo       string `json:"repo"`
	Quant      string `json:"quant"`
	Kind       string `json:"kind,omitempty"`
}

type Manager struct {
	log        *slog.Logger
	catalog    *catalog.Catalog
	downloader *hf.Downloader
	workers    []workerclient.Client
	cfg        Config

	mu      sync.Mutex
	running map[string]workerclient.Status
}

func New(log *slog.Logger, cat *catalog.Catalog, dl *hf.Downloader, workers []workerclient.Client, cfg Config) *Manager {
	return &Manager{log: log, catalog: cat, downloader: dl, workers: workers, cfg: cfg, running: map[string]workerclient.Status{}}
}

func (m *Manager) Reconcile(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = map[string]workerclient.Status{}
	for _, w := range m.workers {
		st, err := w.Status(ctx)
		if err != nil {
			m.log.Warn("worker status failed", "worker", w.BaseURL, "error", err)
			continue
		}
		if st.State == "running" && st.ModelRef != "" {
			m.running[st.ModelRef] = st
		}
	}
}

func (m *Manager) ListModels(ctx context.Context) []ModelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ModelStatus, 0, len(m.catalog.Models))
	for _, cm := range m.catalog.Models {
		_, running := m.running[cm.Ref()]
		downloaded := fileExists(cm.StablePath(m.cfg.ModelsDir))
		kind := cm.Kind
		if kind == "" {
			kind = "chat"
		}
		out = append(out, ModelStatus{ID: cm.Ref(), Object: "model", OwnedBy: "llama.cpp-stack", Downloaded: downloaded, Running: running, ColdStart: !running, Repo: cm.Repo, Quant: cm.Quant, Kind: kind})
	}
	return out
}

func (m *Manager) EnsureRunning(ctx context.Context, ref string) (RunningBackend, error) {
	cm, ok := m.catalog.ByRef(ref)
	if !ok {
		return RunningBackend{}, ErrModelNotFound
	}
	// Serialize download/allocation/load to keep v1 state simple and avoid
	// duplicate downloads or double-loading the same model.
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.running[cm.Ref()]; ok && st.InferenceURL != "" {
		return RunningBackend{ModelRef: cm.Ref(), WorkerID: st.ID, InferenceURL: st.InferenceURL}, nil
	}

	var idle *workerclient.Client
	for i := range m.workers {
		w := m.workers[i]
		st, err := w.Status(ctx)
		if err != nil {
			m.log.Warn("worker status failed during allocation", "worker", w.BaseURL, "error", err)
			continue
		}
		if st.State == "running" && st.ModelRef != "" {
			m.running[st.ModelRef] = st
			if st.ModelRef == cm.Ref() && st.InferenceURL != "" {
				return RunningBackend{ModelRef: cm.Ref(), WorkerID: st.ID, InferenceURL: st.InferenceURL}, nil
			}
			continue
		}
		if st.State == "idle" && idle == nil {
			idle = &m.workers[i]
		}
	}
	if idle == nil {
		return RunningBackend{}, ErrNoIdleWorker
	}

	modelPath, err := m.downloader.Ensure(ctx, cm)
	if err != nil {
		return RunningBackend{}, fmt.Errorf("ensure model file: %w", err)
	}
	rel, err := filepath.Rel(m.cfg.ModelsDir, modelPath)
	if err != nil {
		rel = cm.StableRelPath()
	}
	rel = filepath.ToSlash(rel)
	loadCtx := ctx
	if m.cfg.StartTimeout > 0 {
		var cancel context.CancelFunc
		loadCtx, cancel = context.WithTimeout(ctx, m.cfg.StartTimeout)
		defer cancel()
	}
	resp, err := idle.Load(loadCtx, workerclient.LoadRequest{ModelRef: cm.Ref(), ModelPath: rel, ModelName: cm.Ref(), CtxSize: m.cfg.CtxSize, Parallel: m.cfg.Parallel, ThreadsHTTP: m.cfg.ThreadsHTTP, NGPULayers: m.cfg.NGPULayers, ExtraArgs: m.cfg.ExtraArgs, TimeoutSec: int(m.cfg.StartTimeout.Seconds())})
	if err != nil {
		return RunningBackend{}, fmt.Errorf("load worker %s: %w", idle.ID, err)
	}
	newSt := workerclient.Status{ID: resp.Worker, State: "running", ModelRef: cm.Ref(), ModelPath: rel, ModelName: cm.Ref(), InferenceURL: resp.InferenceURL}
	m.running[cm.Ref()] = newSt
	return RunningBackend{ModelRef: cm.Ref(), WorkerID: resp.Worker, InferenceURL: resp.InferenceURL}, nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Size() > 0
}
