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

// LoadDefaults reads the [defaults.claude] and [defaults.codex] tables of a
// config.toml. A missing file gives no defaults.
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
