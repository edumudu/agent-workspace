package domain

import (
	"reflect"
	"testing"
)

func TestParsePathInput(t *testing.T) {
	cases := []struct {
		name, input string
		want        PathInput
	}{
		{"a child of the base", "./web", PathInput{Dir: "/code/shop", Prefix: "web", Path: "/code/shop/web"}},
		{"a bare name is relative too", "we", PathInput{Dir: "/code/shop", Prefix: "we", Path: "/code/shop/we"}},
		{"a trailing slash lists the folder itself", "./web/", PathInput{Dir: "/code/shop/web", Prefix: "", Path: "/code/shop/web"}},
		{"two levels up", "../../", PathInput{Dir: "/", Prefix: "", Path: "/"}},
		{"up and into a sibling", "../ap", PathInput{Dir: "/code", Prefix: "ap", Path: "/code/ap"}},
		{"dot dot alone is the parent", "..", PathInput{Dir: "/code/shop", Prefix: "..", Path: "/code"}},
		{"an absolute path ignores the base", "/srv/data/x", PathInput{Dir: "/srv/data", Prefix: "x", Path: "/srv/data/x"}},
		{"tilde slash is home", "~/src/a", PathInput{Dir: "/home/me/src", Prefix: "a", Path: "/home/me/src/a"}},
		{"tilde alone lists home", "~", PathInput{Dir: "/home/me", Prefix: "", Path: "/home/me"}},
		{"a hidden prefix stays a prefix", "./.co", PathInput{Dir: "/code/shop", Prefix: ".co", Path: "/code/shop/.co"}},
		{"doubled slashes collapse", ".//web//", PathInput{Dir: "/code/shop/web", Prefix: "", Path: "/code/shop/web"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParsePathInput(c.input, "/code/shop", "/home/me"); got != c.want {
				t.Errorf("ParsePathInput(%q) = %+v, want %+v", c.input, got, c.want)
			}
		})
	}
}

func TestCompleteDirs(t *testing.T) {
	children := []Child{
		{Name: "web", Path: "/r/web", Git: GitDir},
		{Name: "api", Path: "/r/api", Git: GitDir},
		{Name: ".config", Path: "/r/.config"},
		{Name: "Apps", Path: "/r/Apps"},
		{Name: "docs", Path: "/r/docs"},
	}
	names := func(cs []Child) []string {
		out := []string{}
		for _, c := range cs {
			out = append(out, c.Name)
		}
		return out
	}
	cases := []struct {
		name, prefix string
		want         []string
	}{
		{"no prefix lists every visible folder by name", "", []string{"api", "Apps", "docs", "web"}},
		{"a prefix matches regardless of case", "ap", []string{"api", "Apps"}},
		{"a dot prefix shows hidden folders", ".", []string{".config"}},
		{"nothing matches", "zz", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := names(CompleteDirs(children, c.prefix)); !reflect.DeepEqual(got, c.want) {
				t.Errorf("CompleteDirs(%q) = %v, want %v", c.prefix, got, c.want)
			}
		})
	}
}
