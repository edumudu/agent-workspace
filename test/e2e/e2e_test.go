package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var binDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "agentws-e2e")
	if err != nil {
		panic(err)
	}
	build := exec.Command("go", "build",
		"-ldflags", "-X main.version=v0.0.0-e2e -X main.commit=e2ecommit",
		"-o", filepath.Join(dir, "agentws"), "../../cmd/agentws")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		os.Exit(1)
	}
	binDir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			env.Setenv("PATH", binDir+string(os.PathListSeparator)+env.Getenv("PATH"))
			env.Setenv("AGENTWS_HOME", filepath.Join(env.WorkDir, ".agentws"))
			return nil
		},
	})
}
