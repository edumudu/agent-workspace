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

// Recipe is what a repo wants done to a new worktree. Copy and Link paths are
// relative to the repo root and land at the same path in the worktree.
type Recipe struct {
	Copy []string
	Link []string
	Run  []string
	Deps DepsMode
}

// Validate rejects paths that could reach outside the checkout and unknown
// deps modes, so a recipe cannot write or read anywhere else.
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

// Lockfile identifies a dependency lockfile by name and content hash. The zero
// value means the directory has none.
type Lockfile struct {
	Name string
	Hash string
}

// LockfileNames is the order lockfiles are looked for.
var LockfileNames = []string{"bun.lock", "bun.lockb", "pnpm-lock.yaml", "yarn.lock", "package-lock.json"}

var installCommands = map[string][]string{
	"bun.lock":          {"bun", "install", "--frozen-lockfile"},
	"bun.lockb":         {"bun", "install", "--frozen-lockfile"},
	"pnpm-lock.yaml":    {"pnpm", "install", "--frozen-lockfile"},
	"yarn.lock":         {"yarn", "install", "--frozen-lockfile"},
	"package-lock.json": {"npm", "ci"},
}

// InstallCommand is the install for a lockfile's package manager, or nil when
// the lockfile is not one we know.
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

// PlanDeps decides how a worktree gets its dependencies. A clone is only safe
// when the worktree's lockfile is the one main was installed from, so any
// difference falls back to a real install and says why.
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
