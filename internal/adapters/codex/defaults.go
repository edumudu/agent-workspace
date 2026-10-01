package codex

import (
	"errors"
	"io/fs"

	"github.com/BurntSushi/toml"
)

// ReadDefaults reads the model and reasoning effort Codex starts with from
// its config.toml. A missing file or key gives "".
func ReadDefaults(path string) (model, effort string, err error) {
	var c struct {
		Model  string `toml:"model"`
		Effort string `toml:"model_reasoning_effort"`
	}
	_, err = toml.DecodeFile(path, &c)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", nil
	}
	return c.Model, c.Effort, err
}
