package launchd

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
)

type Runner func(ctx context.Context, name string, args ...string) error

func Exec(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, bytes.TrimSpace(out))
	}
	return nil
}

type Agent struct {
	Dir     string
	Label   string
	Program []string
	Env     map[string]string
	Log     string
	UID     int
}

type Result struct {
	Changed bool
	Path    string
	Backup  string
}

func (a Agent) path() string    { return filepath.Join(a.Dir, a.Label+".plist") }
func (a Agent) domain() string  { return "gui/" + strconv.Itoa(a.UID) }
func (a Agent) service() string { return a.domain() + "/" + a.Label }

func Install(ctx context.Context, a Agent, run Runner) (Result, error) {
	res := Result{Path: a.path()}
	want := render(a)
	old, err := os.ReadFile(res.Path)
	switch {
	case err == nil && bytes.Equal(old, want):
		if loaded(ctx, a, run) {
			return res, nil
		}
		return res, run(ctx, "launchctl", "bootstrap", a.domain(), res.Path)
	case err == nil:
		res.Backup = res.Path + ".bak"
		if err := os.WriteFile(res.Backup, old, 0o644); err != nil {
			return res, err
		}
	case !errors.Is(err, os.ErrNotExist):
		return res, err
	}
	if err := os.MkdirAll(a.Dir, 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(res.Path, want, 0o644); err != nil {
		return res, err
	}
	res.Changed = true
	_ = run(ctx, "launchctl", "bootout", a.service())
	return res, run(ctx, "launchctl", "bootstrap", a.domain(), res.Path)
}

func Remove(ctx context.Context, a Agent, run Runner) (bool, error) {
	if _, err := os.Stat(a.path()); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if loaded(ctx, a, run) {
		if err := run(ctx, "launchctl", "bootout", a.service()); err != nil {
			return false, err
		}
	}
	return true, os.Remove(a.path())
}

func loaded(ctx context.Context, a Agent, run Runner) bool {
	return run(ctx, "launchctl", "print", a.service()) == nil
}

func render(a Agent) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	str := func(indent, s string) {
		b.WriteString(indent + "<string>")
		_ = xml.EscapeText(&b, []byte(s))
		b.WriteString("</string>\n")
	}
	key := func(indent, k string) { b.WriteString(indent + "<key>" + k + "</key>\n") }
	key("\t", "Label")
	str("\t", a.Label)
	key("\t", "ProgramArguments")
	b.WriteString("\t<array>\n")
	for _, arg := range a.Program {
		str("\t\t", arg)
	}
	b.WriteString("\t</array>\n")
	if len(a.Env) > 0 {
		key("\t", "EnvironmentVariables")
		b.WriteString("\t<dict>\n")
		names := make([]string, 0, len(a.Env))
		for k := range a.Env {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			b.WriteString("\t\t<key>")
			_ = xml.EscapeText(&b, []byte(k))
			b.WriteString("</key>\n")
			str("\t\t", a.Env[k])
		}
		b.WriteString("\t</dict>\n")
	}
	key("\t", "RunAtLoad")
	b.WriteString("\t<true/>\n")
	key("\t", "KeepAlive")
	b.WriteString("\t<true/>\n")
	if a.Log != "" {
		key("\t", "StandardOutPath")
		str("\t", a.Log)
		key("\t", "StandardErrorPath")
		str("\t", a.Log)
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}
