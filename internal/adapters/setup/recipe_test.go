package setup_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/adapters/setup"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRecipeLoadParsesSetupTable(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".agentws.toml"), `
[setup]
copy = [".env.example"]
link = [".cache"]
run = ["bun run codegen"]
deps = "clone"
`)
	got, found, err := setup.Recipes{}.Load(repo)
	if err != nil || !found {
		t.Fatalf("Load = %v, %v", found, err)
	}
	want := domain.Recipe{
		Copy: []string{".env.example"},
		Link: []string{".cache"},
		Run:  []string{"bun run codegen"},
		Deps: domain.DepsClone,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("recipe = %+v, want %+v", got, want)
	}
}

func TestRecipeLoadWithoutFileIsNotAnError(t *testing.T) {
	_, found, err := setup.Recipes{}.Load(t.TempDir())
	if err != nil || found {
		t.Errorf("Load = %v, %v, want no recipe and no error", found, err)
	}
}

func TestRecipeLoadRejectsMistakes(t *testing.T) {
	cases := map[string]string{
		"not toml":    "copy = [",
		"unknown key": "[setup]\ncopys = [\"a\"]\n",
		"wrong type":  "[setup]\ncopy = \"a\"\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			writeFile(t, filepath.Join(repo, ".agentws.toml"), body)
			_, _, err := setup.Recipes{}.Load(repo)
			if err == nil || !strings.Contains(err.Error(), ".agentws.toml") {
				t.Errorf("err = %v, want one naming the file", err)
			}
		})
	}
}

func TestRecipeLockfilePicksTheKnownOneAndHashesContent(t *testing.T) {
	a, b, none := t.TempDir(), t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(a, "pnpm-lock.yaml"), "one")
	writeFile(t, filepath.Join(b, "pnpm-lock.yaml"), "two")
	writeFile(t, filepath.Join(a, "notes.lock"), "ignored")

	la, err := setup.FS{}.Lockfile(a)
	if err != nil {
		t.Fatal(err)
	}
	lb, _ := setup.FS{}.Lockfile(b)
	ln, err := setup.FS{}.Lockfile(none)
	if err != nil {
		t.Fatal(err)
	}
	if la.Name != "pnpm-lock.yaml" || la.Hash == "" || la.Hash == lb.Hash {
		t.Errorf("a = %+v, b = %+v, want the same name and different hashes", la, lb)
	}
	if ln != (domain.Lockfile{}) {
		t.Errorf("none = %+v, want the zero lockfile", ln)
	}
}
