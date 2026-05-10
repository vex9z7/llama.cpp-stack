package preset

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vex9z7/llama.cpp-stack/internal/catalog"
)

type Config struct {
	ModelsDir   string
	Path        string
	CtxSize     int
	Parallel    int
	ThreadsHTTP int
	NGPULayers  int
	ExtraArgs   string
}

type Rendered struct {
	Path          string
	IncludedRefs  []string
	IncludedCount int
}

func Render(cat *catalog.Catalog, cfg Config) (Rendered, error) {
	if cfg.Path == "" {
		cfg.Path = filepath.Join(cfg.ModelsDir, "models-preset.generated.ini")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return Rendered{}, err
	}
	var b strings.Builder
	b.WriteString("version = 1\n\n")
	b.WriteString("[*]\n")
	writeInt(&b, "ctx-size", cfg.CtxSize)
	writeInt(&b, "parallel", cfg.Parallel)
	writeInt(&b, "threads-http", cfg.ThreadsHTTP)
	writeInt(&b, "n-gpu-layers", cfg.NGPULayers)
	b.WriteString("jinja = true\n")
	for _, arg := range strings.Fields(cfg.ExtraArgs) {
		// Extra args are kept for future explicit support. Avoid trying to parse arbitrary shell here.
		_ = arg
	}
	b.WriteString("\n")

	models := append([]catalog.Model(nil), cat.Models...)
	sort.SliceStable(models, func(i, j int) bool { return models[i].Ref() < models[j].Ref() })
	included := make([]string, 0, len(models))
	for _, m := range models {
		stable := m.StablePath(cfg.ModelsDir)
		st, err := os.Stat(stable)
		if err != nil || st.IsDir() || st.Size() == 0 {
			continue
		}
		ref := m.Ref()
		included = append(included, ref)
		b.WriteString("[")
		b.WriteString(ref)
		b.WriteString("]\n")
		b.WriteString("model = ")
		b.WriteString(stable)
		b.WriteString("\n")
		b.WriteString("alias = ")
		b.WriteString(ref)
		b.WriteString("\n")
		if kindOf(m) == "embedding" {
			b.WriteString("embeddings = true\n")
			b.WriteString("pooling = mean\n")
		}
		b.WriteString("\n")
	}
	tmp := cfg.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return Rendered{}, err
	}
	if err := os.Rename(tmp, cfg.Path); err != nil {
		return Rendered{}, err
	}
	return Rendered{Path: cfg.Path, IncludedRefs: included, IncludedCount: len(included)}, nil
}

func writeInt(b *strings.Builder, key string, val int) {
	if val == 0 {
		return
	}
	fmt.Fprintf(b, "%s = %d\n", key, val)
}

func kindOf(m catalog.Model) string {
	if m.Kind == "" {
		return "chat"
	}
	return m.Kind
}
