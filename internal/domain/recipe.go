package domain

import (
	"fmt"
	"path"
	"strings"
)

type DepsMode string

const (
	DepsNone    DepsMode = ""
	DepsClone   DepsMode = "clone"
	DepsLink    DepsMode = "link"
	DepsInstall DepsMode = "install"
)

type Recipe struct {
	Copy []string
	Link []string
	Run  []string
	Deps DepsMode
}

// why: so a recipe cannot write or read outside the checkout.
func (r Recipe) Validate() error {
	for _, p := range r.Copy {
		if err := checkRecipePath(p); err != nil {
			return fmt.Errorf("copy %q: %w", p, err)
		}
	}
	for _, p := range r.Link {
		if err := checkRecipePath(p); err != nil {
			return fmt.Errorf("link %q: %w", p, err)
		}
	}
	for _, c := range r.Run {
		if strings.TrimSpace(c) == "" {
			return fmt.Errorf("run: blank command")
		}
	}
	switch r.Deps {
	case DepsNone, DepsClone, DepsLink, DepsInstall:
		return nil
	}
	return fmt.Errorf("deps %q: want clone, link or install", r.Deps)
}

func checkRecipePath(p string) error {
	clean := path.Clean(p)
	switch {
	case strings.TrimSpace(p) == "":
		return fmt.Errorf("empty path")
	case path.IsAbs(p):
		return fmt.Errorf("must be relative to the repo")
	case clean == ".." || strings.HasPrefix(clean, "../"):
		return fmt.Errorf("must stay inside the repo")
	}
	return nil
}

type Lockfile struct {
	Name string
	Hash string
}

var LockfileNames = []string{"bun.lock", "bun.lockb", "pnpm-lock.yaml", "yarn.lock", "package-lock.json"}

var installCommands = map[string][]string{
	"bun.lock":          {"bun", "install", "--frozen-lockfile"},
	"bun.lockb":         {"bun", "install", "--frozen-lockfile"},
	"pnpm-lock.yaml":    {"pnpm", "install", "--frozen-lockfile"},
	"yarn.lock":         {"yarn", "install", "--frozen-lockfile"},
	"package-lock.json": {"npm", "ci"},
}

func InstallCommand(lockfile string) []string {
	return installCommands[lockfile]
}

type DepsAction string

const (
	DepsSkip         DepsAction = "skip"
	DepsCloneModules DepsAction = "clone"
	DepsLinkModules  DepsAction = "link"
	DepsRunInstall   DepsAction = "install"
)

type DepsInput struct {
	MainHasModules bool
	Main           Lockfile
	Worktree       Lockfile
}

type DepsPlan struct {
	Action  DepsAction
	Command []string
	Reason  string
}

const (
	reasonNoModules  = "the main checkout has no node_modules"
	reasonNoLockfile = "no known lockfile in the worktree, nothing to install from"
)

// why: a clone is only safe when the worktree's lockfile is the one main was
// installed from, so any difference falls back to a real install.
func PlanDeps(mode DepsMode, in DepsInput) DepsPlan {
	switch mode {
	case DepsInstall:
		return installPlan(in, "")
	case DepsLink:
		if !in.MainHasModules {
			return DepsPlan{Action: DepsSkip, Reason: reasonNoModules}
		}
		return DepsPlan{Action: DepsLinkModules}
	case DepsClone:
		switch {
		case !in.MainHasModules:
			return installPlan(in, reasonNoModules)
		case in.Main != in.Worktree:
			return installPlan(in, in.Worktree.Name+" differs from the main checkout")
		}
		return DepsPlan{Action: DepsCloneModules}
	}
	return DepsPlan{Action: DepsSkip}
}

func installPlan(in DepsInput, reason string) DepsPlan {
	command := InstallCommand(in.Worktree.Name)
	if command == nil {
		return DepsPlan{Action: DepsSkip, Reason: reasonNoLockfile}
	}
	return DepsPlan{Action: DepsRunInstall, Command: command, Reason: reason}
}
