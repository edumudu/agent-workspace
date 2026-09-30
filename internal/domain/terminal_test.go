package domain

import "testing"

func TestChooseShell(t *testing.T) {
	api := Worktree{ID: "w-api", Path: "/wt/api", SessionID: "s1", Repo: "/src/api", SubtaskSlug: "part-1"}
	web := Worktree{ID: "w-web", Path: "/wt/web", SessionID: "s1", Repo: "/src/web", Branch: "feat-web"}
	other := Worktree{ID: "w-other", Path: "/wt/other", SessionID: "s2"}
	cases := []struct {
		name      string
		worktrees []Worktree
		wanted    string
		cwd       string
		want      ShellTarget
		ok        bool
	}{
		{"the wanted worktree", []Worktree{api, web}, "w-web", "/root", ShellTarget{Key: "s1/w-web", Dir: "/wt/web", Label: "web:feat-web"}, true},
		{"the first of several when none is wanted", []Worktree{web, api}, "", "/root", ShellTarget{Key: "s1/w-api", Dir: "/wt/api", Label: "api:part-1"}, true},
		{"a worktree of another session is not offered", []Worktree{other, web}, "", "/root", ShellTarget{Key: "s1/w-web", Dir: "/wt/web", Label: "web:feat-web"}, true},
		{"a wanted worktree the session does not own", []Worktree{api, other}, "w-other", "/root", ShellTarget{}, false},
		{"no worktrees falls back to the session directory", []Worktree{other}, "", "/root", ShellTarget{Key: "s1/root", Dir: "/root", Label: "root"}, true},
		{"no worktrees and no directory", nil, "", "", ShellTarget{}, false},
		{"a wanted worktree that is unknown even with a directory", nil, "w-api", "/root", ShellTarget{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ChooseShell("s1", c.worktrees, c.wanted, c.cwd)
			if got != c.want || ok != c.ok {
				t.Errorf("ChooseShell = %+v, %v; want %+v, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestNvimOpenExpr(t *testing.T) {
	cases := []struct {
		name string
		path string
		line int
		want string
	}{
		{"a plain path", "/wt/api/main.go", 12, `execute('edit +12 ' . fnameescape('/wt/api/main.go'))`},
		{"a quote in the path is doubled", "/wt/it's/a.go", 3, `execute('edit +3 ' . fnameescape('/wt/it''s/a.go'))`},
		{"no line opens at the top", "/wt/a.go", 0, `execute('edit +1 ' . fnameescape('/wt/a.go'))`},
		{"a negative line opens at the top", "/wt/a.go", -4, `execute('edit +1 ' . fnameescape('/wt/a.go'))`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NvimOpenExpr(c.path, c.line); got != c.want {
				t.Errorf("NvimOpenExpr = %s; want %s", got, c.want)
			}
		})
	}
}

func TestResolveCommentFile(t *testing.T) {
	root := Worktree{ID: "w-root", Path: "/wt/api", SessionID: "s1"}
	nested := Worktree{ID: "w-nested", Path: "/wt/api/pkg/sub", SessionID: "s1"}
	other := Worktree{ID: "w-other", Path: "/wt/web", SessionID: "s2"}
	cases := []struct {
		name      string
		worktrees []Worktree
		file      string
		wantID    string
		wantPath  string
		ok        bool
	}{
		{"a file in a worktree", []Worktree{root, other}, "/wt/api/cmd/main.go", "w-root", "cmd/main.go", true},
		{"the innermost worktree wins", []Worktree{root, nested}, "/wt/api/pkg/sub/x.go", "w-nested", "x.go", true},
		{"a worktree of another session does not match", []Worktree{other}, "/wt/web/a.go", "", "", false},
		{"a sibling with the same prefix does not match", []Worktree{root}, "/wt/api-old/a.go", "", "", false},
		{"a relative file never matches", []Worktree{root}, "cmd/main.go", "", "", false},
		{"the worktree directory itself is not a file", []Worktree{root}, "/wt/api", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wt, rel, ok := ResolveCommentFile("s1", c.worktrees, c.file)
			if wt.ID != c.wantID || rel != c.wantPath || ok != c.ok {
				t.Errorf("ResolveCommentFile = %q, %q, %v; want %q, %q, %v", wt.ID, rel, ok, c.wantID, c.wantPath, c.ok)
			}
		})
	}
}

func TestDraftCommentRange(t *testing.T) {
	cases := []struct {
		name       string
		start, end int
		wantStart  int
		wantEnd    int
		ok         bool
	}{
		{"one line", 4, 4, 4, 4, true},
		{"a range", 4, 9, 4, 9, true},
		{"a reversed range is put in order", 9, 4, 4, 9, true},
		{"an end of zero means one line", 7, 0, 7, 7, true},
		{"line zero is not a line", 0, 0, 0, 0, false},
		{"a negative line is not a line", -1, 3, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end, ok := NormalizeLines(c.start, c.end)
			if start != c.wantStart || end != c.wantEnd || ok != c.ok {
				t.Errorf("NormalizeLines(%d, %d) = %d, %d, %v; want %d, %d, %v", c.start, c.end, start, end, ok, c.wantStart, c.wantEnd, c.ok)
			}
		})
	}
}

func TestShellTitleNamesTheWorktreeItsPathAndTheKeys(t *testing.T) {
	got := ShellTitle(ShellTarget{Key: "s1/w", Dir: "/wt/api", Label: "api:part-1"})
	if want := "shell · api:part-1 · /wt/api · t hide · s type · T popup"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
