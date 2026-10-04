package omp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"slices"
	"time"
)

// why: providers change while a file is still young, so a caller that waits always asks omp again. A caller that does not wait returns the file, and refreshes a stale one aside, because `omp models` is slow.
func LoadCached(ctx context.Context, path, binary string, maxAge time.Duration, block bool) []string {
	ids, fresh := readCatalogFile(path, maxAge)
	if fresh && !block {
		return ids
	}
	if !block {
		go func() { _, _ = refreshCatalog(context.Background(), path, binary) }()
		return ids
	}
	next, err := refreshCatalog(ctx, path, binary)
	if err != nil {
		return ids
	}
	return next
}

func FetchCatalog(ctx context.Context, binary string) ([]string, error) {
	if binary == "" {
		binary = "omp"
	}
	out, err := exec.CommandContext(ctx, binary, "models", "--json", "--no-extensions").Output()
	if err != nil {
		return nil, err
	}
	return ParseCatalog(out)
}

func ParseCatalog(data []byte) ([]string, error) {
	var doc struct {
		Models []struct {
			Selector string `json:"selector"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(doc.Models))
	for _, m := range doc.Models {
		if m.Selector != "" {
			out = append(out, m.Selector)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func refreshCatalog(ctx context.Context, path, binary string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ids, err := FetchCatalog(ctx, binary)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.New("omp models: empty catalog")
	}
	body, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(path, body); err != nil {
		return ids, nil
	}
	return ids, nil
}

func readCatalogFile(path string, maxAge time.Duration) ([]string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var ids []string
	if err := json.Unmarshal(body, &ids); err != nil {
		return nil, false
	}
	return ids, time.Since(info.ModTime()) < maxAge
}
