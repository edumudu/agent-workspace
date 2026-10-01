package domain

import (
	"strings"
	"testing"
)

func TestOnboardingNeededOnlyWhenSomethingIsLeft(t *testing.T) {
	on := HarnessSetup{Installed: true}
	cases := []struct {
		name string
		o    Onboarding
		want bool
	}{
		{"fresh machine", Onboarding{Nvim: NvimSetup{OnPath: true}}, true},
		{"done", Onboarding{Done: true}, false},
		{"claude set up, no nvim", Onboarding{Claude: on}, false},
		{"codex set up, nvim configured", Onboarding{Codex: on, Nvim: NvimSetup{OnPath: true, Configured: true}}, false},
		{"claude set up, nvim not configured", Onboarding{Claude: on, Nvim: NvimSetup{OnPath: true}}, true},
		{"no harness, nvim configured", Onboarding{Nvim: NvimSetup{OnPath: true, Configured: true}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OnboardingNeeded(c.o); got != c.want {
				t.Errorf("needed = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAddNvimBlockIsIdempotentAndRemovable(t *testing.T) {
	cases := []struct {
		name, config, file string
	}{
		{"empty init.lua", "", "/c/init.lua"},
		{"existing init.lua", "vim.o.number = true\n", "/c/init.lua"},
		{"no trailing newline", "vim.o.number = true", "/c/init.lua"},
		{"init.vim", "set number\n", "/c/init.vim"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			added, changed := AddNvimBlock(c.config, "/p/nvim", c.file)
			if !changed || !strings.HasPrefix(added, c.config) {
				t.Fatalf("add = %q, %v", added, changed)
			}
			for _, l := range NvimSnippet("/p/nvim", c.file) {
				if !strings.Contains(added, l+"\n") {
					t.Errorf("block lacks %q:\n%s", l, added)
				}
			}
			if again, changed := AddNvimBlock(added, "/p/nvim", c.file); changed || again != added {
				t.Errorf("second add changed it:\n%s", again)
			}
			removed, changed := RemoveNvimBlock(added)
			want := c.config
			if want != "" && !strings.HasSuffix(want, "\n") {
				want += "\n"
			}
			if !changed || removed != want {
				t.Errorf("remove = %q, %v; want %q", removed, changed, want)
			}
			if _, changed := RemoveNvimBlock(c.config); changed {
				t.Error("remove without a block changed it")
			}
		})
	}
}

func TestAddNvimBlockReplacesAStaleBlock(t *testing.T) {
	old, _ := AddNvimBlock("x = 1\n", "/old/nvim", "init.lua")
	got, changed := AddNvimBlock(old, "/new/nvim", "init.lua")
	if !changed || strings.Contains(got, "/old/nvim") || !strings.Contains(got, "/new/nvim") || strings.Count(got, "agentws:begin") != 1 {
		t.Errorf("replace = %q, %v", got, changed)
	}
}

func TestNvimBlockLinesAreWhatIsWritten(t *testing.T) {
	block := NvimBlock("/p/nvim", "init.vim")
	if block[0] != `" agentws:begin (agentws setup nvim --remove takes this out)` || block[len(block)-1] != `" agentws:end` {
		t.Errorf("vim markers = %q", block)
	}
	lua := NvimBlock("/p/nvim", "init.lua")
	if !strings.HasPrefix(lua[0], "-- agentws:begin") || lua[len(lua)-1] != "-- agentws:end" {
		t.Errorf("lua markers = %q", lua)
	}
}
