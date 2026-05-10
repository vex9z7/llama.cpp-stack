package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Catalog struct {
	Models []Model `toml:"models"`
}

type Model struct {
	Repo    string `toml:"repo" json:"repo"`
	Quant   string `toml:"quant" json:"quant"`
	Pattern string `toml:"pattern,omitempty" json:"pattern,omitempty"`
	File    string `toml:"file,omitempty" json:"file,omitempty"`
	Name    string `toml:"name,omitempty" json:"name,omitempty"`
	Kind    string `toml:"kind,omitempty" json:"kind,omitempty"`
}

func Load(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	for i := range c.Models {
		if err := c.Models[i].Validate(); err != nil {
			return nil, fmt.Errorf("model %d: %w", i, err)
		}
	}
	return &c, nil
}

func (m Model) Validate() error {
	if strings.Count(m.Repo, "/") != 1 {
		return fmt.Errorf("repo must be owner/name: %q", m.Repo)
	}
	if m.Quant == "" {
		return fmt.Errorf("quant is required for repo %q", m.Repo)
	}
	if strings.Contains(m.Quant, "/") {
		return fmt.Errorf("quant must not contain slash: %q", m.Quant)
	}
	return nil
}

func (m Model) Ref() string { return m.Repo + "/" + m.Quant }

func (m Model) DisplayName() string {
	if m.Name != "" {
		return m.Name
	}
	return m.Ref()
}

func (m Model) GlobPattern() string {
	if m.File != "" {
		return m.File
	}
	if m.Pattern != "" {
		return m.Pattern
	}
	return "*" + m.Quant + "*.gguf"
}

func (m Model) StableRelPath() string {
	return filepath.ToSlash(filepath.Join("hf", filepath.FromSlash(m.Repo), m.Quant+".gguf"))
}

func (m Model) StablePath(modelsDir string) string {
	return filepath.Join(modelsDir, filepath.FromSlash(m.StableRelPath()))
}

func (c *Catalog) ByRef(ref string) (Model, bool) {
	for _, m := range c.Models {
		if m.Ref() == ref || m.DisplayName() == ref {
			return m, true
		}
	}
	return Model{}, false
}
