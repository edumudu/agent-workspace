//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pluginDir is the nvim/ Lua plugin, two levels up from this package.
func pluginDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "nvim"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "lua", "agentws", "init.lua")); err != nil {
		t.Fatalf("the nvim plugin is missing: %v", err)
	}
	return dir
}

// TestNvimPluginSpecs runs nvim/test/spec.lua in a headless nvim with a
// throwaway config, data, state and cache dir: the user's nvim setup is never
// read or written.
func TestNvimPluginSpecs(t *testing.T) {
	plugin := pluginDir(t)
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not installed")
	}
	home := t.TempDir()
	cmd := exec.Command("nvim", "--clean", "--headless", "-n", "-i", "NONE", "-l", filepath.Join(plugin, "test", "spec.lua"))
	cmd.Env = append(os.Environ(),
		"AGENTWS_NVIM_PLUGIN="+plugin,
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"XDG_DATA_HOME="+filepath.Join(home, "data"),
		"XDG_STATE_HOME="+filepath.Join(home, "state"),
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"),
		"AGENTWS_SESSION=",
		"TMUX=",
	)
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("nvim specs failed: %v", err)
	}
	if !strings.Contains(string(out), "all specs passed") {
		t.Fatalf("the specs did not finish:\n%s", out)
	}
}
