package claude

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// ReadDefaults reads the model and effort Claude Code starts with from its
// settings file. A missing file or key gives "".
func ReadDefaults(path string) (model, effort string, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	var s struct {
		Model       string `json:"model"`
		EffortLevel string `json:"effortLevel"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return "", "", err
	}
	return s.Model, s.EffortLevel, nil
}
