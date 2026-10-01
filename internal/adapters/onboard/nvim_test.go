package onboard_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallNvimWritesTheBlockBacksUpAndRemoves(t *testing.T) {
	w := newWorld(t, true)
	if err := os.MkdirAll(w.probe.PluginDirs[0], 0o755); err != nil {
		t.Fatal(err)
	}
	init := filepath.Join(w.probe.NvimConfigDir, "init.lua")
	write(t, init, "vim.o.number = true\n")
	ctx := context.Background()

	n, err := w.probe.InstallNvim(ctx)
	if err != nil || !n.Configured || n.Backup != init+".agentws-backup" {
		t.Fatalf("install = %+v, %v", n, err)
	}
	got := read(t, init)
	if !strings.HasPrefix(got, "vim.o.number = true\n") || !strings.Contains(got, "prepend('"+w.probe.PluginDirs[0]+"')") {
		t.Errorf("init.lua = %s", got)
	}
	if read(t, n.Backup) != "vim.o.number = true\n" {
		t.Errorf("backup = %s", read(t, n.Backup))
	}
	if _, err := w.probe.InstallNvim(ctx); err != nil || read(t, init) != got {
		t.Errorf("second install changed init.lua: %v", err)
	}
	if read(t, n.Backup) != "vim.o.number = true\n" {
		t.Error("second install overwrote the backup")
	}

	removed, err := w.probe.RemoveNvim(ctx)
	if err != nil || !removed || read(t, init) != "vim.o.number = true\n" {
		t.Errorf("remove = %v, %v: %s", removed, err, read(t, init))
	}
	if removed, _ := w.probe.RemoveNvim(ctx); removed {
		t.Error("second remove reported a change")
	}
}

func TestInstallNvimCreatesInitLuaOrUsesInitVim(t *testing.T) {
	w := newWorld(t, true)
	if err := os.MkdirAll(w.probe.PluginDirs[0], 0o755); err != nil {
		t.Fatal(err)
	}
	n, err := w.probe.InstallNvim(context.Background())
	init := filepath.Join(w.probe.NvimConfigDir, "init.lua")
	if err != nil || n.ConfigFile != init || n.Backup != "" || !strings.Contains(read(t, init), "require('agentws').setup({})") {
		t.Errorf("new init.lua: %+v, %v", n, err)
	}

	v := newWorld(t, true)
	if err := os.MkdirAll(v.probe.PluginDirs[0], 0o755); err != nil {
		t.Fatal(err)
	}
	vim := filepath.Join(v.probe.NvimConfigDir, "init.vim")
	write(t, vim, "set number\n")
	if _, err := v.probe.InstallNvim(context.Background()); err != nil || !strings.Contains(read(t, vim), "lua << EOF") {
		t.Errorf("init.vim: %v\n%s", err, read(t, vim))
	}
	if _, err := os.Stat(filepath.Join(v.probe.NvimConfigDir, "init.lua")); !os.IsNotExist(err) {
		t.Error("an init.lua was created next to init.vim")
	}
}

func TestInstallNvimRefusesWithoutThePluginFiles(t *testing.T) {
	w := newWorld(t, true)
	if _, err := w.probe.InstallNvim(context.Background()); err == nil {
		t.Error("installed a block pointing at a missing plugin dir")
	}
	if _, err := os.Stat(filepath.Join(w.probe.NvimConfigDir, "init.lua")); !os.IsNotExist(err) {
		t.Error("init.lua written anyway")
	}
}
