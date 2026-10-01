package domain

import "strings"

type HarnessSetup struct {
	Installed bool   `json:"installed"`
	File      string `json:"file"`
	Backup    string `json:"backup,omitempty"`
	Err       string `json:"err,omitempty"`
}

// why: PluginFound tells a plugin dir the snippet can point at from one a source build lacks.
type NvimSetup struct {
	OnPath      bool   `json:"on_path"`
	Configured  bool   `json:"configured"`
	ConfigFile  string `json:"config_file"`
	PluginDir   string `json:"plugin_dir"`
	PluginFound bool   `json:"plugin_found"`
}

type Onboarding struct {
	Done   bool         `json:"done"`
	Claude HarnessSetup `json:"claude"`
	Codex  HarnessSetup `json:"codex"`
	Nvim   NvimSetup    `json:"nvim"`
}

type OnboardStep string

const (
	OnboardPick   OnboardStep = "pick"
	OnboardClaude OnboardStep = "claude"
	OnboardCodex  OnboardStep = "codex"
	OnboardNvim   OnboardStep = "nvim"
	OnboardFinish OnboardStep = "finish"
)

// why: Codex keeps a hash of every hook it was told to trust, so new hooks stay off until the user trusts them.
const CodexTrustStep = "Codex runs a hook only after you trust it. Start codex and accept the review prompt for the new agentws hooks, or open /hooks and trust them there."

func OnboardSteps(claude, codex bool) []OnboardStep {
	steps := []OnboardStep{OnboardPick}
	if claude {
		steps = append(steps, OnboardClaude)
	}
	if codex {
		steps = append(steps, OnboardCodex)
	}
	return append(steps, OnboardNvim, OnboardFinish)
}

// why: a step not in steps goes to finish, so a stale step never loops.
func NextOnboardStep(steps []OnboardStep, cur OnboardStep) OnboardStep {
	for i, s := range steps {
		if s == cur && i+1 < len(steps) {
			return steps[i+1]
		}
	}
	return OnboardFinish
}

func DefaultOnboardPicks(o Onboarding) (claude, codex bool) {
	claude, codex = o.Claude.Installed, o.Codex.Installed
	if !claude && !codex {
		claude = true
	}
	return claude, codex
}

type SetupOffer int

const (
	OfferInstall SetupOffer = iota
	OfferInstalled
	// why: a config agentws cannot read gets no install offer, so it is never overwritten.
	OfferBroken
)

func HarnessOffer(h HarnessSetup) SetupOffer {
	switch {
	case h.Err != "":
		return OfferBroken
	case h.Installed:
		return OfferInstalled
	}
	return OfferInstall
}

type NvimOffer int

const (
	NvimMissing NvimOffer = iota
	NvimReady
	NvimShowSnippet
	// why: a build that did not come from a release archive has no plugin files.
	NvimNoPlugin
)

func NvimOfferFor(n NvimSetup) NvimOffer {
	switch {
	case !n.OnPath:
		return NvimMissing
	case n.Configured:
		return NvimReady
	case n.PluginFound:
		return NvimShowSnippet
	}
	return NvimNoPlugin
}

// why: an init.vim cannot hold lua directly, so it gets a heredoc.
func NvimSnippet(pluginDir, configFile string) []string {
	quoted := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(pluginDir)
	lua := []string{
		"vim.opt.runtimepath:prepend('" + quoted + "')",
		"require('agentws').setup({})",
	}
	if strings.HasSuffix(configFile, ".vim") {
		return append(append([]string{"lua << EOF"}, lua...), "EOF")
	}
	return lua
}
