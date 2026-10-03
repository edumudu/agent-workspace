package domain

import (
	"path/filepath"
	"sort"
	"strings"
)

type PathInput struct {
	Dir    string
	Prefix string
	Path   string
}

func ParsePathInput(input, base, home string) PathInput {
	switch {
	case input == "~":
		input = home + "/"
	case strings.HasPrefix(input, "~/"):
		input = home + input[1:]
	case !filepath.IsAbs(input):
		input = base + "/" + input
	}
	cut := strings.LastIndex(input, "/")
	return PathInput{
		Dir:    filepath.Clean(input[:cut+1]),
		Prefix: input[cut+1:],
		Path:   filepath.Clean(input),
	}
}

func CompleteDirs(children []Child, prefix string) []Child {
	lower := strings.ToLower(prefix)
	out := []Child{}
	for _, c := range children {
		if strings.HasPrefix(c.Name, ".") != strings.HasPrefix(prefix, ".") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(c.Name), lower) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}
