// why: usage: lint-agents [-max n] [root] keeps the root AGENTS.md small and the
// scoped ones loadable by Claude Code (a CLAUDE.md symlink beside each), and fails
// on relative links in them or ARCHITECTURE.md that do not resolve (ADR 0041).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	agentsFile = "AGENTS.md"
	claudeFile = "CLAUDE.md"
	archFile   = "ARCHITECTURE.md"
)

var (
	linkPattern = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	codeSpan    = regexp.MustCompile("`[^`]*`")
)

func main() {
	maxLines := flag.Int("max", 150, "line limit for the root AGENTS.md")
	flag.Parse()
	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}
	os.Exit(run(root, *maxLines, os.Stderr))
}

func run(root string, maxLines int, out io.Writer) int {
	findings, err := check(root, maxLines)
	if err != nil {
		fmt.Fprintln(out, err)
		return 2
	}
	for _, f := range findings {
		fmt.Fprintln(out, f)
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

func check(root string, maxLines int) ([]string, error) {
	var findings []string
	rootAgents, err := os.ReadFile(filepath.Join(root, agentsFile))
	if err != nil {
		return nil, err
	}
	if n := bytes.Count(rootAgents, []byte("\n")); n > maxLines {
		findings = append(findings, fmt.Sprintf("%s: %d lines, over the %d-line limit", agentsFile, n, maxLines))
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		switch {
		case d.Name() == agentsFile:
			findings = append(findings, symlinkFinding(path, rel)...)
		case rel == archFile:
		default:
			return nil
		}
		broken, err := brokenLinks(path, rel)
		findings = append(findings, broken...)
		return err
	})
	return findings, err
}

func skipDir(name string) bool {
	switch name {
	case ".git", ".claude", "testdata", "node_modules", "vendor", "bin", "dist":
		return true
	}
	return false
}

func symlinkFinding(agentsPath, agentsRel string) []string {
	claudeRel := filepath.Join(filepath.Dir(agentsRel), claudeFile)
	target, err := os.Readlink(filepath.Join(filepath.Dir(agentsPath), claudeFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return []string{fmt.Sprintf("%s: missing, want a symlink to %s", claudeRel, agentsFile)}
	case err != nil || target != agentsFile:
		return []string{fmt.Sprintf("%s: not a symlink to %s", claudeRel, agentsFile)}
	}
	return nil
}

func brokenLinks(path, rel string) ([]string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var findings []string
	for i, line := range strings.Split(string(src), "\n") {
		for _, m := range linkPattern.FindAllStringSubmatch(codeSpan.ReplaceAllString(line, ""), -1) {
			target := m[1]
			if external(target) {
				continue
			}
			file, _, _ := strings.Cut(target, "#")
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), file)); err != nil {
				findings = append(findings, fmt.Sprintf("%s:%d: broken link %s", rel, i+1, target))
			}
		}
	}
	return findings, nil
}

func external(target string) bool {
	return strings.HasPrefix(target, "#") || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:")
}
