package llamaprocess

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type LoadRequest struct {
	ModelRef    string `json:"model_ref"`
	ModelPath   string `json:"model_path"`
	ModelName   string `json:"model_name"`
	CtxSize     int    `json:"ctx_size"`
	Parallel    int    `json:"parallel"`
	ThreadsHTTP int    `json:"threads_http"`
	NGPULayers  int    `json:"n_gpu_layers"`
	ExtraArgs   string `json:"extra_args"`
	TimeoutSec  int    `json:"timeout_seconds"`
	Embeddings  bool   `json:"embeddings"`
}

type Status struct {
	ID            string `json:"id"`
	State         string `json:"state"`
	ModelRef      string `json:"model_ref,omitempty"`
	ModelPath     string `json:"model_path,omitempty"`
	ModelName     string `json:"model_name,omitempty"`
	InferenceURL  string `json:"inference_url,omitempty"`
	PID           int    `json:"pid,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	LoadedAt      string `json:"loaded_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	StderrTail    string `json:"stderr_tail,omitempty"`
	Embeddings    bool   `json:"embeddings,omitempty"`
	LastExitError string `json:"last_exit_error,omitempty"`
}

type Supervisor struct {
	WorkerID      string
	ModelsDir     string
	ServerBin     string
	ServerHost    string
	ServerPort    string
	InferenceURL  string
	DefaultTimout time.Duration

	mu     sync.Mutex
	cmd    *exec.Cmd
	status Status
	stderr *tailBuffer
}

func New(workerID, modelsDir, serverBin, serverHost, serverPort string, timeout time.Duration) *Supervisor {
	if serverHost == "" {
		serverHost = "0.0.0.0"
	}
	if serverPort == "" {
		serverPort = "8080"
	}
	return &Supervisor{WorkerID: workerID, ModelsDir: modelsDir, ServerBin: serverBin, ServerHost: serverHost, ServerPort: serverPort, InferenceURL: "http://" + workerID + ":" + serverPort, DefaultTimout: timeout, status: Status{ID: workerID, State: "idle"}}
}

func (s *Supervisor) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	if s.stderr != nil {
		st.StderrTail = s.stderr.String()
	}
	return st
}

func (s *Supervisor) Load(ctx context.Context, req LoadRequest) (Status, error) {
	s.mu.Lock()
	if s.cmd != nil && s.cmd.Process != nil {
		s.mu.Unlock()
		return Status{}, errors.New("worker already has a loaded model")
	}
	bin := resolveBin(s.ServerBin)
	modelAbs := filepath.Join(s.ModelsDir, filepath.FromSlash(req.ModelPath))
	if st, err := os.Stat(modelAbs); err != nil || st.IsDir() || st.Size() == 0 {
		s.mu.Unlock()
		if err != nil {
			return Status{}, fmt.Errorf("model file not available: %w", err)
		}
		return Status{}, fmt.Errorf("model file not available: %s", modelAbs)
	}
	args := []string{
		"--host", s.ServerHost,
		"--port", s.ServerPort,
		"--model", modelAbs,
		"--alias", req.ModelName,
		"--ctx-size", fmt.Sprint(req.CtxSize),
		"--parallel", fmt.Sprint(req.Parallel),
		"--threads-http", fmt.Sprint(req.ThreadsHTTP),
		"--n-gpu-layers", fmt.Sprint(req.NGPULayers),
		"--cont-batching",
		"--cache-prompt",
	}
	if req.Embeddings {
		args = append(args, "--embeddings")
	}
	args = append(args, splitArgs(req.ExtraArgs)...)
	cmd := exec.CommandContext(context.Background(), bin, args...)
	stderr := newTailBuffer(16 * 1024)
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, stderr)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		s.status.LastError = err.Error()
		s.mu.Unlock()
		return Status{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s.cmd = cmd
	s.stderr = stderr
	s.status = Status{ID: s.WorkerID, State: "starting", ModelRef: req.ModelRef, ModelPath: req.ModelPath, ModelName: req.ModelName, InferenceURL: s.InferenceURL, PID: cmd.Process.Pid, StartedAt: now, Embeddings: req.Embeddings}
	s.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		if s.cmd == cmd {
			last := s.status
			s.cmd = nil
			s.status = Status{ID: s.WorkerID, State: "idle", LastError: last.LastError, StderrTail: stderr.String()}
			if err != nil {
				s.status.LastExitError = err.Error()
			}
		}
		s.mu.Unlock()
		done <- err
	}()

	timeout := s.DefaultTimout
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := s.waitReady(readyCtx, done); err != nil {
		s.mu.Lock()
		if s.status.ID == s.WorkerID {
			s.status.LastError = err.Error()
		}
		s.mu.Unlock()
		_ = s.Unload(context.Background(), true)
		return Status{}, fmt.Errorf("llama-server did not become ready: %w; stderr_tail=%s", err, stderr.String())
	}
	s.mu.Lock()
	s.status.State = "running"
	s.status.LoadedAt = time.Now().UTC().Format(time.RFC3339)
	st := s.status
	st.StderrTail = stderr.String()
	s.mu.Unlock()
	return st, nil
}

func (s *Supervisor) Unload(ctx context.Context, force bool) error {
	s.mu.Lock()
	cmd := s.cmd
	if cmd == nil || cmd.Process == nil {
		s.status = Status{ID: s.WorkerID, State: "idle", LastError: s.status.LastError, StderrTail: s.status.StderrTail, LastExitError: s.status.LastExitError}
		s.mu.Unlock()
		return nil
	}
	pid := cmd.Process.Pid
	s.mu.Unlock()
	if !force {
		busy, err := s.busy(ctx)
		if err == nil && busy {
			return errors.New("worker has active llama-server slots")
		}
	}
	if runtime.GOOS != "windows" {
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	} else {
		_ = cmd.Process.Signal(os.Interrupt)
	}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		s.mu.Lock()
		exited := s.cmd != cmd
		s.mu.Unlock()
		if exited {
			return nil
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			if runtime.GOOS != "windows" {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			} else {
				_ = cmd.Process.Kill()
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Supervisor) waitReady(ctx context.Context, exited <-chan error) error {
	url := "http://127.0.0.1:" + s.ServerPort + "/health"
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-exited:
			if err == nil {
				err = errors.New("llama-server exited before becoming ready")
			}
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_, _ = ioCopyDiscard(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					return nil
				}
			}
		}
	}
}

func (s *Supervisor) busy(ctx context.Context) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+s.ServerPort+"/slots", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return false, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	text := string(body)
	return strings.Contains(text, "\"is_processing\":true") || strings.Contains(text, "\"is_processing\": true") || strings.Contains(text, "is_processing: true"), nil
}

func resolveBin(bin string) string {
	if bin != "" {
		return bin
	}
	candidates := []string{"/app/llama-server", "/usr/local/bin/llama-server", "/usr/bin/llama-server", "llama-server"}
	for _, c := range candidates {
		if strings.Contains(c, "/") {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c
			}
		} else if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return "/app/llama-server"
}

func splitArgs(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Fields(s)
}

func ioCopyDiscard(r io.Reader) (int64, error) {
	return io.Copy(io.Discard, r)
}

type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newTailBuffer(max int) *tailBuffer { return &tailBuffer{max: max} }

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(bytes.Clone(b.buf)))
}
