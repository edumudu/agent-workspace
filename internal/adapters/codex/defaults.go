package codex

import (
	"errors"
	"io/fs"

	"github.com/BurntSushi/toml"
)

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
