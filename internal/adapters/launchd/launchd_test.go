package launchd_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/adapters/launchd"
)

type launchctl struct{ calls [][]string }

func (l *launchctl) run(_ context.Context, name string, args ...string) error {
	l.calls = append(l.calls, append([]string{name}, args...))
	return nil
}

func agent(dir string) launchd.Agent {
	return launchd.Agent{
		Dir:     dir,
		Label:   "dev.agentws.bridge.me-vps",
		Program: []string{"/usr/local/bin/agentws", "notify", "bridge", "me@vps"},
		Env:     map[string]string{"PATH": "/opt/homebrew/bin:/usr/bin", "AGENTWS_HOME": "/Users/me/.agentws"},
		Log:     "/Users/me/.agentws/bridge-me@vps.log",
		UID:     501,
	}
}

func TestLaunchdInstallWritesAKeptAliveAgentAndLoadsIt(t *testing.T) {
	dir := t.TempDir()
	l := &launchctl{}
	res, err := launchd.Install(context.Background(), agent(dir), l.run)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "dev.agentws.bridge.me-vps.plist")
	if !res.Changed || res.Path != path || res.Backup != "" {
		t.Fatalf("result %+v", res)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plist := string(raw)
	for _, want := range []string{
		"<key>Label</key>\n\t<string>dev.agentws.bridge.me-vps</string>",
		"<string>/usr/local/bin/agentws</string>\n\t\t<string>notify</string>\n\t\t<string>bridge</string>\n\t\t<string>me@vps</string>",
		"<key>AGENTWS_HOME</key>\n\t\t<string>/Users/me/.agentws</string>\n\t\t<key>PATH</key>\n\t\t<string>/opt/homebrew/bin:/usr/bin</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>KeepAlive</key>\n\t<true/>",
		"<key>StandardErrorPath</key>\n\t<string>/Users/me/.agentws/bridge-me@vps.log</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist lacks %q:\n%s", want, plist)
		}
	}
	want := [][]string{
		{"launchctl", "bootout", "gui/501/dev.agentws.bridge.me-vps"},
		{"launchctl", "bootstrap", "gui/501", path},
	}
	if !reflect.DeepEqual(l.calls, want) {
		t.Fatalf("launchctl calls %q", l.calls)
	}
}

func TestLaunchdInstallEscapesXML(t *testing.T) {
	dir := t.TempDir()
	a := agent(dir)
	a.Program = []string{"/opt/a&b/agentws", "<x>"}
	if _, err := launchd.Install(context.Background(), a, (&launchctl{}).run); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, a.Label+".plist"))
	if !strings.Contains(string(raw), "<string>/opt/a&amp;b/agentws</string>") || !strings.Contains(string(raw), "<string>&lt;x&gt;</string>") {
		t.Fatalf("not escaped:\n%s", raw)
	}
}

func TestLaunchdInstallTwiceChangesNothing(t *testing.T) {
	dir := t.TempDir()
	if _, err := launchd.Install(context.Background(), agent(dir), (&launchctl{}).run); err != nil {
		t.Fatal(err)
	}
	l := &launchctl{}
	res, err := launchd.Install(context.Background(), agent(dir), l.run)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || len(l.calls) != 0 {
		t.Fatalf("result %+v calls %q", res, l.calls)
	}
}

func TestLaunchdInstallOverADifferentAgentBacksItUpAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.agentws.bridge.me-vps.plist")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := &launchctl{}
	res, err := launchd.Install(context.Background(), agent(dir), l.run)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Backup == "" || len(l.calls) != 2 {
		t.Fatalf("result %+v calls %q", res, l.calls)
	}
	if old, err := os.ReadFile(res.Backup); err != nil || string(old) != "old" {
		t.Fatalf("backup %q, %v", old, err)
	}
}

func TestLaunchdInstallFailsWhenItCannotLoad(t *testing.T) {
	failing := func(_ context.Context, _ string, args ...string) error {
		if args[0] == "bootstrap" {
			return errors.New("bootstrap failed")
		}
		return errors.New("not loaded")
	}
	if _, err := launchd.Install(context.Background(), agent(t.TempDir()), failing); err == nil {
		t.Fatal("a failed bootstrap was not reported")
	}
}

func TestLaunchdRemoveUnloadsAndDeletes(t *testing.T) {
	dir := t.TempDir()
	a := agent(dir)
	if _, err := launchd.Install(context.Background(), a, (&launchctl{}).run); err != nil {
		t.Fatal(err)
	}
	l := &launchctl{}
	removed, err := launchd.Remove(context.Background(), a, l.run)
	if err != nil || !removed {
		t.Fatalf("removed %v err %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, a.Label+".plist")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plist still there: %v", err)
	}
	if want := [][]string{{"launchctl", "bootout", "gui/501/" + a.Label}}; !reflect.DeepEqual(l.calls, want) {
		t.Fatalf("calls %q", l.calls)
	}
}

func TestLaunchdRemoveWithoutAnAgentIsANoop(t *testing.T) {
	l := &launchctl{}
	removed, err := launchd.Remove(context.Background(), agent(t.TempDir()), l.run)
	if err != nil || removed || len(l.calls) != 0 {
		t.Fatalf("removed %v err %v calls %q", removed, err, l.calls)
	}
}
