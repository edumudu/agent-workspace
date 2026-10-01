package claude

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

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
	return shortModel(s.Model, s.Model), s.EffortLevel, nil
}

func ReadEffortByModel(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s struct {
		ModelSettings map[string]struct {
			EffortLevel string `json:"effortLevel"`
		} `json:"modelSettings"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for id, m := range s.ModelSettings {
		if m.EffortLevel != "" {
			out[shortModel(id, id)] = m.EffortLevel
		}
	}
	return out, nil
}
