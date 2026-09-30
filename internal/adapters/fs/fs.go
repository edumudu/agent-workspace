// Package fs implements app.WorkspaceFS on the local filesystem.
package fs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

var _ app.WorkspaceFS = FS{}

type FS struct{}

// Marker reports what path/.git is. It fails when path is not a directory.
func (FS) Marker(path string) (domain.GitMarker, error) {
	info, err := os.Stat(path)
	if err != nil {
		return domain.GitNone, err
	}
	if !info.IsDir() {
		return domain.GitNone, fmt.Errorf("not a directory")
	}
	return marker(path), nil
}

func marker(dir string) domain.GitMarker {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	switch {
	case err != nil:
		return domain.GitNone
	case info.IsDir():
		return domain.GitDir
	default:
		return domain.GitFile
	}
}

// Children lists the directories directly under path, following symlinks. An
// entry that cannot be stat'ed, such as a dangling link, is skipped.
func (FS) Children(path string) ([]domain.Child, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var out []domain.Child
	for _, e := range entries {
		child := filepath.Join(path, e.Name())
		info, err := os.Stat(child)
		if err != nil || !info.IsDir() {
			continue
		}
		out = append(out, domain.Child{Name: e.Name(), Path: child, Git: marker(child)})
	}
	return out, nil
}
