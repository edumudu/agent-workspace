package tui

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/BurntSushi/toml"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// Defaults is what the new-session dialog starts a harness with. An empty
// field leaves the harness's own default.
type Defaults struct {
	Model  string `toml:"model"`
	Effort string `toml:"effort"`
}

// LoadDefaults gives no defaults when the file is missing.
func LoadDefaults(path string) (map[domain.Harness]Defaults, error) {
	var cfg struct {
		Defaults map[string]Defaults `toml:"defaults"`
	}
	_, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tui: %s: %w", path, err)
	}
	out := map[domain.Harness]Defaults{}
	for _, h := range []domain.Harness{domain.HarnessClaude, domain.HarnessCodex} {
		if d, ok := cfg.Defaults[string(h)]; ok {
			out[h] = d
		}
	}
	return out, nil
}

// LoadFallback reads the [fallback] table: the low-quota threshold and the
// Claude-to-Codex model and effort maps. A missing file gives the zero config.
func LoadFallback(path string) (domain.FallbackConfig, error) {
	var cfg struct {
		Fallback struct {
			Threshold int               `toml:"threshold"`
			Models    map[string]string `toml:"models"`
			Efforts   map[string]string `toml:"efforts"`
		} `toml:"fallback"`
	}
	_, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return domain.FallbackConfig{}, nil
	}
	if err != nil {
		return domain.FallbackConfig{}, fmt.Errorf("tui: %s: %w", path, err)
	}
	f := cfg.Fallback
	return domain.FallbackConfig{Threshold: f.Threshold, Models: f.Models, Efforts: f.Efforts}, nil
}
