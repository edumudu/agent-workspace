package git

import "testing"

func TestParseStatus(t *testing.T) {
	cases := []struct {
		name        string
		out         string
		wantBranch  string
		wantChanged int
	}{
		{"clean", "# branch.oid abc\x00# branch.head main\x00", "main", 0},
		{"detached has no branch", "# branch.oid abc\x00# branch.head (detached)\x00", "", 0},
		{
			"ordinary, untracked and unmerged each count once",
			"# branch.head feat\x001 .M N... 100644 100644 100644 a b file.go\x00? new.txt\x00u UU N... 1 2 3 4 a b c conflict.go\x00",
			"feat", 3,
		},
		{
			"a rename's original path is not a second file",
			"# branch.head feat\x002 R. N... 100644 100644 100644 a b R100 new.go\x00old.go\x00? x\x00",
			"feat", 2,
		},
		{"ignored files are not changes", "# branch.head main\x00! ignored.log\x00", "main", 0},
		{"empty output", "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			branch, changed := parseStatus([]byte(c.out))
			if branch != c.wantBranch || changed != c.wantChanged {
				t.Errorf("parseStatus = (%q, %d), want (%q, %d)", branch, changed, c.wantBranch, c.wantChanged)
			}
		})
	}
}

func TestParseDefaultBranch(t *testing.T) {
	cases := map[string]string{
		"origin/main\n":        "main",
		"origin/release/1.x\n": "release/1.x",
		"upstream/main\n":      "main",
		"":                     "",
		"\n":                   "",
	}
	for in, want := range cases {
		if got := parseDefaultBranch(in); got != want {
			t.Errorf("parseDefaultBranch(%q) = %q, want %q", in, got, want)
		}
	}
}
