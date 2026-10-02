package loginshell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func profileHome(t *testing.T, profile string) {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
}

func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shell")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoginPathReadsWhatTheProfileAdds(t *testing.T) {
	profileHome(t, "echo welcome back\nPATH=\"$PATH:/opt/extra/bin\"\nprintf 'no newline'\n")
	got, err := Path(context.Background(), "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, ":/opt/extra/bin") || strings.Contains(got, "welcome") || strings.Contains(got, "newline") {
		t.Fatalf("Path = %q, want the profile's PATH alone", got)
	}
}

func TestLoginPathFallsBackToBinShWithoutShell(t *testing.T) {
	profileHome(t, "PATH=\"$PATH:/opt/extra/bin\"\n")
	got, err := Path(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, ":/opt/extra/bin") {
		t.Fatalf("Path = %q, want the profile's PATH", got)
	}
}

func TestLoginPathFailsWhenTheShellDoes(t *testing.T) {
	for name, body := range map[string]string{
		"exit status": "exit 3",
		"no answer":   "exit 0",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := Path(context.Background(), script(t, body)); err == nil {
				t.Fatalf("Path = %q, want an error", got)
			}
		})
	}
}

func TestLoginPathGivesUpOnAHangingShell(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Path(ctx, script(t, "sleep 30 & sleep 30"))
	if err == nil {
		t.Fatal("Path returned no error for a shell that never answers")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("Path took %v, want it to stop soon after the deadline", took)
	}
}
