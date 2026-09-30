package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestRecipeValidate(t *testing.T) {
	cases := []struct {
		name    string
		recipe  Recipe
		wantErr string
	}{
		{"empty is fine", Recipe{}, ""},
		{"relative paths and known deps", Recipe{Copy: []string{".env.example", "config/local.json"}, Link: []string{".cache"}, Deps: DepsClone}, ""},
		{"absolute copy path", Recipe{Copy: []string{"/etc/passwd"}}, "copy"},
		{"copy escapes the checkout", Recipe{Copy: []string{"../secrets"}}, "copy"},
		{"link escapes through a subdir", Recipe{Link: []string{"a/../../b"}}, "link"},
		{"absolute link path", Recipe{Link: []string{"/tmp/x"}}, "link"},
		{"empty path", Recipe{Copy: []string{""}}, "copy"},
		{"unknown deps mode", Recipe{Deps: "hardlink"}, "deps"},
		{"blank run command", Recipe{Run: []string{"  "}}, "run"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.recipe.Validate()
			switch {
			case c.wantErr == "" && err != nil:
				t.Errorf("Validate() = %v, want nil", err)
			case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
				t.Errorf("Validate() = %v, want an error mentioning %q", err, c.wantErr)
			}
		})
	}
}

func TestRecipeInstallCommand(t *testing.T) {
	cases := []struct {
		lockfile string
		want     []string
	}{
		{"bun.lock", []string{"bun", "install", "--frozen-lockfile"}},
		{"bun.lockb", []string{"bun", "install", "--frozen-lockfile"}},
		{"pnpm-lock.yaml", []string{"pnpm", "install", "--frozen-lockfile"}},
		{"yarn.lock", []string{"yarn", "install", "--frozen-lockfile"}},
		{"package-lock.json", []string{"npm", "ci"}},
		{"Cargo.lock", nil},
		{"", nil},
	}
	for _, c := range cases {
		t.Run(c.lockfile, func(t *testing.T) {
			if got := InstallCommand(c.lockfile); !reflect.DeepEqual(got, c.want) {
				t.Errorf("InstallCommand(%q) = %v, want %v", c.lockfile, got, c.want)
			}
		})
	}
}

func TestRecipePlanDeps(t *testing.T) {
	bun := Lockfile{Name: "bun.lock", Hash: "aaa"}
	bunInstall := []string{"bun", "install", "--frozen-lockfile"}
	cases := []struct {
		name string
		mode DepsMode
		in   DepsInput
		want DepsPlan
	}{
		{"no mode does nothing", DepsNone, DepsInput{MainHasModules: true, Main: bun, Worktree: bun}, DepsPlan{Action: DepsSkip}},
		{
			"clone when the lockfile matches",
			DepsClone, DepsInput{MainHasModules: true, Main: bun, Worktree: bun},
			DepsPlan{Action: DepsCloneModules},
		},
		{
			"clone when neither side has a lockfile",
			DepsClone, DepsInput{MainHasModules: true},
			DepsPlan{Action: DepsCloneModules},
		},
		{
			"install when the hash differs",
			DepsClone, DepsInput{MainHasModules: true, Main: bun, Worktree: Lockfile{Name: "bun.lock", Hash: "bbb"}},
			DepsPlan{Action: DepsRunInstall, Command: bunInstall, Reason: "bun.lock differs from the main checkout"},
		},
		{
			"install when the worktree added a lockfile",
			DepsClone, DepsInput{MainHasModules: true, Worktree: bun},
			DepsPlan{Action: DepsRunInstall, Command: bunInstall, Reason: "bun.lock differs from the main checkout"},
		},
		{
			"install when the manager changed",
			DepsClone, DepsInput{MainHasModules: true, Main: bun, Worktree: Lockfile{Name: "pnpm-lock.yaml", Hash: "aaa"}},
			DepsPlan{Action: DepsRunInstall, Command: []string{"pnpm", "install", "--frozen-lockfile"}, Reason: "pnpm-lock.yaml differs from the main checkout"},
		},
		{
			"skip when the worktree lost its lockfile",
			DepsClone, DepsInput{MainHasModules: true, Main: bun},
			DepsPlan{Action: DepsSkip, Reason: "no known lockfile in the worktree, nothing to install from"},
		},
		{
			"install when main has no modules",
			DepsClone, DepsInput{Main: bun, Worktree: bun},
			DepsPlan{Action: DepsRunInstall, Command: bunInstall, Reason: "the main checkout has no node_modules"},
		},
		{
			"install mode always installs",
			DepsInstall, DepsInput{MainHasModules: true, Main: bun, Worktree: bun},
			DepsPlan{Action: DepsRunInstall, Command: bunInstall},
		},
		{
			"install without a lockfile is skipped with a reason",
			DepsInstall, DepsInput{},
			DepsPlan{Action: DepsSkip, Reason: "no known lockfile in the worktree, nothing to install from"},
		},
		{
			"link when main has modules",
			DepsLink, DepsInput{MainHasModules: true},
			DepsPlan{Action: DepsLinkModules},
		},
		{
			"link is skipped when main has no modules",
			DepsLink, DepsInput{},
			DepsPlan{Action: DepsSkip, Reason: "the main checkout has no node_modules"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PlanDeps(c.mode, c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("PlanDeps() = %+v, want %+v", got, c.want)
			}
		})
	}
}
