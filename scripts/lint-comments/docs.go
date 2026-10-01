package main

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"unicode"
)

// why: filler words carry no information in a doc comment: after dropping them and
// every word taken from the declaration's own identifiers, a useful doc still
// has something left.
var fillerWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "be": true,
	"return": true, "new": true, "create": true, "get": true, "set": true,
	"of": true, "for": true, "given": true, "which": true, "that": true,
	"this": true, "it": true, "to": true, "and": true, "or": true, "with": true,
	"hold": true, "contain": true, "represent": true, "describe": true,
	"define": true, "value": true, "type": true, "struct": true,
	"method": true, "function": true, "func": true, "on": true, "in": true,
	"by": true, "from": true, "its": true,
}

var (
	wordPattern    = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*|[0-9]+`)
	bannerRun      = regexp.MustCompile(`[-=*#/~_]{3,}`)
	directiveStart = regexp.MustCompile(`^//(go:|nolint|lint:|\s*\+build)`)
)

var fillerStems = func() map[string]bool {
	stems := map[string]bool{}
	for w := range fillerWords {
		stems[stem(w)] = true
	}
	return stems
}()

func restatingDocs(fset *token.FileSet, file string, f *ast.File) []finding {
	var findings []finding
	check := func(doc *ast.CommentGroup, name string, related []string) {
		if doc == nil || !restates(doc.Text(), name, related) {
			return
		}
		findings = append(findings, finding{file, fset.Position(doc.Pos()).Line,
			"doc comment only restates " + name + "; say what the signature does not, or delete it"})
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			var related []string
			if d.Recv != nil {
				related = append(related, fieldWords(d.Recv)...)
			}
			related = append(related, fieldWords(d.Type.Params)...)
			related = append(related, fieldWords(d.Type.Results)...)
			check(d.Doc, d.Name.Name, related)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				doc := specDoc(spec)
				if doc == nil && !d.Lparen.IsValid() {
					doc = d.Doc
				}
				for _, name := range specNames(spec) {
					check(doc, name, specWords(spec))
					break
				}
			}
		}
	}
	return findings
}

func specDoc(spec ast.Spec) *ast.CommentGroup {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return s.Doc
	case *ast.ValueSpec:
		return s.Doc
	}
	return nil
}

func specNames(spec ast.Spec) []string {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return []string{s.Name.Name}
	case *ast.ValueSpec:
		var names []string
		for _, n := range s.Names {
			names = append(names, n.Name)
		}
		return names
	}
	return nil
}

func specWords(spec ast.Spec) []string {
	var words []string
	switch s := spec.(type) {
	case *ast.TypeSpec:
		words = identWords(s.Type)
	case *ast.ValueSpec:
		for _, n := range s.Names {
			words = append(words, n.Name)
		}
		if s.Type != nil {
			words = append(words, identWords(s.Type)...)
		}
	}
	return words
}

func fieldWords(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var words []string
	for _, field := range fields.List {
		for _, n := range field.Names {
			words = append(words, n.Name)
		}
		words = append(words, identWords(field.Type)...)
	}
	return words
}

// why: only top-level identifiers count: a struct's field names are what its
// doc comment may talk about.
func identWords(expr ast.Expr) []string {
	switch e := expr.(type) {
	case *ast.Ident:
		return []string{e.Name}
	case *ast.StarExpr:
		return identWords(e.X)
	case *ast.SelectorExpr:
		return append(identWords(e.X), e.Sel.Name)
	case *ast.ArrayType:
		return identWords(e.Elt)
	case *ast.MapType:
		return append(identWords(e.Key), identWords(e.Value)...)
	case *ast.ChanType:
		return identWords(e.Value)
	case *ast.Ellipsis:
		return identWords(e.Elt)
	case *ast.IndexExpr:
		return identWords(e.X)
	}
	return nil
}

// why: words are compared after stripping plural and verb endings so
// "Close closes" matches.
func restates(text, name string, related []string) bool {
	words := wordPattern.FindAllString(text, -1)
	if len(words) == 0 || words[0] != name {
		return false
	}
	known := map[string]bool{}
	for _, id := range append([]string{name}, related...) {
		for _, part := range splitIdent(id) {
			known[stem(part)] = true
		}
	}
	for _, w := range words[1:] {
		for _, part := range splitIdent(w) {
			s := stem(part)
			if !known[s] && !fillerStems[s] {
				return false
			}
		}
	}
	return true
}

func splitIdent(id string) []string {
	var parts []string
	runes := []rune(id)
	start := 0
	for i := 1; i < len(runes); i++ {
		lowerToUpper := unicode.IsLower(runes[i-1]) && unicode.IsUpper(runes[i])
		acronymEnd := i+1 < len(runes) && unicode.IsUpper(runes[i-1]) && unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i+1])
		if lowerToUpper || acronymEnd {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	return append(parts, string(runes[start:]))
}

func stem(word string) string {
	w := strings.ToLower(word)
	for _, suffix := range []string{"ing", "ed", "es", "s"} {
		if len(w) > len(suffix)+2 && strings.HasSuffix(w, suffix) {
			w = strings.TrimSuffix(w, suffix)
			break
		}
	}
	return strings.TrimSuffix(w, "e")
}

func banners(fset *token.FileSet, file string, f *ast.File) []finding {
	docs := map[*ast.CommentGroup]bool{f.Doc: true}
	ast.Inspect(f, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.FuncDecl:
			docs[d.Doc] = true
		case *ast.GenDecl:
			docs[d.Doc] = true
		}
		return true
	})
	var findings []finding
	for _, group := range f.Comments {
		if group.Pos() < f.Package || (!docs[group] && insideDecl(fset, group, f.Decls)) {
			continue
		}
		for _, c := range group.List {
			if isSeparator(c.Text) || (!docs[group] && isLabel(c.Text, len(group.List))) {
				findings = append(findings, finding{file, fset.Position(c.Pos()).Line, "section banner comment; delete it"})
			}
		}
	}
	return findings
}

func insideDecl(fset *token.FileSet, group *ast.CommentGroup, decls []ast.Decl) bool {
	line := fset.Position(group.Pos()).Line
	for _, d := range decls {
		if group.Pos() >= d.Pos() && group.End() <= d.End() {
			return true
		}
		if fset.Position(d.End()).Line == line {
			return true
		}
	}
	return false
}

// why: checked on doc comments too: Go attaches a banner that sits
// right above a declaration to it as its doc.
func isSeparator(text string) bool {
	body, ok := commentBody(text)
	return ok && bannerRun.MatchString(body) && len(wordPattern.FindAllString(body, -1)) <= 4
}

func isLabel(text string, groupLen int) bool {
	body, ok := commentBody(text)
	return ok && groupLen == 1 && len(strings.Fields(body)) <= 3 && !strings.ContainsAny(body, ":,;().?!")
}

func commentBody(text string) (string, bool) {
	if directiveStart.MatchString(text) || !strings.HasPrefix(text, "//") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(text, "//")), true
}
