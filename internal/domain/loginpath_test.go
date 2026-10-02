package domain

import "testing"

func TestLoginPathMerge(t *testing.T) {
	cases := []struct {
		name, current, login, want string
	}{
		{"appends what the login shell adds", "/usr/bin:/bin", "/home/u/.local/bin:/usr/bin:/bin", "/usr/bin:/bin:/home/u/.local/bin"},
		{"keeps current order and precedence", "/fakes:/usr/bin", "/usr/bin:/fakes:/opt/bin", "/fakes:/usr/bin:/opt/bin"},
		{"adds each entry once", "/usr/bin", "/a:/a:/usr/bin:/b:/a", "/usr/bin:/a:/b"},
		{"drops empty entries the login shell has", "/usr/bin", "::/a::", "/usr/bin:/a"},
		{"empty login leaves current as is", "/usr/bin::/bin", "", "/usr/bin::/bin"},
		{"empty current takes the login path", "", "/a:/b", "/a:/b"},
		{"nothing new leaves current as is", "/a:/b", "/b:/a", "/a:/b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MergeLoginPath(c.current, c.login); got != c.want {
				t.Fatalf("MergeLoginPath(%q, %q) = %q, want %q", c.current, c.login, got, c.want)
			}
		})
	}
}
