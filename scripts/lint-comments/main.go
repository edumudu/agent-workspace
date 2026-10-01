// why: usage: lint-comments [dir...] fails on any comment that does not open
// with a "why:" or "bug:" marker, an issue-linked to-do, or a generated-code or license
// header, and on tool directives without a reason (AGENTS.md, ADR 0020).
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type finding struct {
	file string
	line int
	msg  string
}

func (f finding) String() string { return fmt.Sprintf("%s:%d: %s", f.file, f.line, f.msg) }

var (
	todoPattern      = regexp.MustCompile(`\bTODO\b`)
	todoIssuePattern = regexp.MustCompile(`\bTODO\(#\d+\)`)
	nolintWithReason = regexp.MustCompile(`^//nolint:\S+\s+//\s*\S`)
)

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}
	os.Exit(run(roots, os.Stderr))
}

func run(roots []string, out io.Writer) int {
	failed := false
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != root && skipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			findings, err := checkFile(path, src)
			if err != nil {
				return err
			}
			for _, f := range findings {
				fmt.Fprintln(out, f)
				failed = true
			}
			return nil
		})
		if err != nil {
			fmt.Fprintln(out, err)
			return 2
		}
	}
	if failed {
		return 1
	}
	return 0
}

func skipDir(name string) bool {
	return name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

var (
	markedPattern    = regexp.MustCompile(`^(//|/\*) (why|bug): \S`)
	todoMarker       = regexp.MustCompile(`^// TODO\(#\d+\)`)
	generatedPattern = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)
	licensePattern   = regexp.MustCompile(`^// (Copyright|SPDX-License-Identifier:)`)
	directivePattern = regexp.MustCompile(`^//(go:|line |export |nolint:)`)
)

const unmarkedMsg = "comment must start with \"// why: \" or \"// bug: \" (a reason the code cannot show), or be a tool directive; otherwise delete it"

func checkFile(name string, src []byte) ([]finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var findings []finding
	checkTODO := func(line int, text string) {
		if todoPattern.MatchString(text) && !todoIssuePattern.MatchString(text) {
			findings = append(findings, finding{name, line, "TODO must reference an issue: TODO(#<n>)"})
		}
	}
	for _, group := range file.Comments {
		needsMarker := true
		for _, c := range group.List {
			line := fset.Position(c.Pos()).Line
			if directivePattern.MatchString(c.Text) {
				if strings.HasPrefix(c.Text, "//nolint:") && !nolintWithReason.MatchString(c.Text) {
					findings = append(findings, finding{name, line, "//nolint: needs a reason: //nolint:x // why: ..."})
				}
				checkTODO(line, c.Text)
				needsMarker = true
				continue
			}
			if needsMarker && !marked(c.Text) {
				findings = append(findings, finding{name, line, unmarkedMsg})
				needsMarker = false
				continue
			}
			needsMarker = false
			checkTODO(line, c.Text)
		}
	}
	slices.SortStableFunc(findings, func(a, b finding) int { return a.line - b.line })
	return findings, nil
}

func marked(text string) bool {
	return markedPattern.MatchString(text) || todoMarker.MatchString(text) ||
		generatedPattern.MatchString(text) || licensePattern.MatchString(text)
}
