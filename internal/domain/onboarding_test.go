package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestOnboardStepsFollowThePickedHarnesses(t *testing.T) {
	cases := []struct {
		name          string
		claude, codex bool
		want          []OnboardStep
	}{
		{"both", true, true, []OnboardStep{OnboardPick, OnboardClaude, OnboardCodex, OnboardNvim, OnboardFinish}},
		{"claude only", true, false, []OnboardStep{OnboardPick, OnboardClaude, OnboardNvim, OnboardFinish}},
		{"codex only", false, true, []OnboardStep{OnboardPick, OnboardCodex, OnboardNvim, OnboardFinish}},
		{"neither", false, false, []OnboardStep{OnboardPick, OnboardNvim, OnboardFinish}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OnboardSteps(c.claude, c.codex); !reflect.DeepEqual(got, c.want) {
				t.Errorf("steps = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNextOnboardStepWalksForwardAndStopsAtFinish(t *testing.T) {
	steps := OnboardSteps(false, true)
	cases := []struct {
		cur, want OnboardStep
	}{
		{OnboardPick, OnboardCodex},
		{OnboardCodex, OnboardNvim},
		{OnboardNvim, OnboardFinish},
		{OnboardFinish, OnboardFinish},
		{OnboardClaude, OnboardFinish},
	}
	for _, c := range cases {
		if got := NextOnboardStep(steps, c.cur); got != c.want {
			t.Errorf("after %s = %s, want %s", c.cur, got, c.want)
		}
	}
}

func TestDefaultOnboardPicksPreferWhatIsAlreadyInstalled(t *testing.T) {
	cases := []struct {
		name          string
		o             Onboarding
		claude, codex bool
	}{
		{"nothing installed picks claude", Onboarding{}, true, false},
		{"codex installed", Onboarding{Codex: HarnessSetup{Installed: true}}, false, true},
		{"both installed", Onboarding{Claude: HarnessSetup{Installed: true}, Codex: HarnessSetup{Installed: true}}, true, true},
		{"claude installed", Onboarding{Claude: HarnessSetup{Installed: true}}, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			claude, codex := DefaultOnboardPicks(c.o)
			if claude != c.claude || codex != c.codex {
				t.Errorf("picks = %v, %v, want %v, %v", claude, codex, c.claude, c.codex)
			}
		})
	}
}

func TestHarnessOfferSaysWhatTheStepCanDo(t *testing.T) {
	cases := []struct {
		name string
		h    HarnessSetup
		want SetupOffer
	}{
		{"not installed", HarnessSetup{File: "/c/settings.json"}, OfferInstall},
		{"installed", HarnessSetup{Installed: true, File: "/c/settings.json"}, OfferInstalled},
		{"unreadable config", HarnessSetup{File: "/c/settings.json", Err: "not valid JSON"}, OfferBroken},
		{"error wins over installed", HarnessSetup{Installed: true, Err: "boom"}, OfferBroken},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HarnessOffer(c.h); got != c.want {
				t.Errorf("offer = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNvimOfferForEachSetup(t *testing.T) {
	cases := []struct {
		name string
		n    NvimSetup
		want NvimOffer
	}{
		{"no nvim", NvimSetup{PluginFound: true}, NvimMissing},
		{"configured", NvimSetup{OnPath: true, Configured: true, PluginFound: true}, NvimReady},
		{"configured without the shipped dir", NvimSetup{OnPath: true, Configured: true}, NvimReady},
		{"plugin shipped, not configured", NvimSetup{OnPath: true, PluginFound: true}, NvimShowSnippet},
		{"plugin dir missing", NvimSetup{OnPath: true}, NvimNoPlugin},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NvimOfferFor(c.n); got != c.want {
				t.Errorf("offer = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNvimSnippetPointsAtThePluginDir(t *testing.T) {
	lua := NvimSnippet("/Users/me/.local/share/agentws/nvim", "/Users/me/.config/nvim/init.lua")
	want := []string{
		"vim.opt.runtimepath:prepend('/Users/me/.local/share/agentws/nvim')",
		"require('agentws').setup({})",
	}
	if !reflect.DeepEqual(lua, want) {
		t.Errorf("lua snippet = %q, want %q", lua, want)
	}

	vim := NvimSnippet("/opt/agentws/nvim", "/home/me/.config/nvim/init.vim")
	if len(vim) != 4 || vim[0] != "lua << EOF" || vim[3] != "EOF" || vim[1] != "vim.opt.runtimepath:prepend('/opt/agentws/nvim')" {
		t.Errorf("vimscript snippet = %q", vim)
	}

	quoted := strings.Join(NvimSnippet("/tmp/it's here", "init.lua"), "\n")
	if !strings.Contains(quoted, `prepend('/tmp/it\'s here')`) {
		t.Errorf("a quote in the path is not escaped: %q", quoted)
	}
}
