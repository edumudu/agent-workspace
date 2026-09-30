// Command lint-comments enforces the comment rules in AGENTS.md: inside
// function bodies only "// why:" comments and tool directives are allowed,
// every to-do marker references an issue number, a declaration's doc comment
// says more than its name, and there are no section banners.
package main

import (
	"fmt"
	"go/ast"
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

func checkFile(name string, src []byte) ([]finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	bodies := functionBodies(file)
	var findings []finding
	for _, group := range file.Comments {
		for _, c := range group.List {
			line := fset.Position(c.Pos()).Line
			if insideAny(c.Pos(), bodies) && !allowedInBody(c.Text) {
				findings = append(findings, finding{name, line, "comment inside a function body must start with \"// why:\" or be a tool directive"})
				continue
			}
			if todoPattern.MatchString(c.Text) && !todoIssuePattern.MatchString(c.Text) {
				findings = append(findings, finding{name, line, "TODO must reference an issue: TODO(#<n>)"})
			}
		}
	}
	findings = append(findings, restatingDocs(fset, name, file)...)
	findings = append(findings, banners(fset, name, file)...)
	slices.SortStableFunc(findings, func(a, b finding) int { return a.line - b.line })
	return findings, nil
}

func functionBodies(file *ast.File) []*ast.BlockStmt {
	var bodies []*ast.BlockStmt
	ast.Inspect(file, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body != nil {
				bodies = append(bodies, fn.Body)
			}
		case *ast.FuncLit:
			bodies = append(bodies, fn.Body)
		}
		return true
	})
	return bodies
}

func insideAny(pos token.Pos, bodies []*ast.BlockStmt) bool {
	for _, b := range bodies {
		if pos > b.Lbrace && pos < b.Rbrace {
			return true
		}
	}
	return false
}

func allowedInBody(text string) bool {
	switch {
	case strings.HasPrefix(text, "// why:"):
		return true
	case strings.HasPrefix(text, "//go:"):
		return true
	case strings.HasPrefix(text, "//nolint:"):
		return nolintWithReason.MatchString(text)
	}
	return false
}
