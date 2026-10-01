package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
)

var binDir string

// waitWithin bounds every poll in the scripts; waits never sleep a fixed time.
const waitWithin = 15 * time.Second

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "agentws-e2e")
	if err != nil {
		panic(err)
	}
	build := exec.Command("go", "build",
		"-ldflags", "-X github.com/giovaniif/agent-workspace/internal/version.Version=v0.0.0-e2e -X github.com/giovaniif/agent-workspace/internal/version.Commit=e2ecommit",
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
	fakes, err := filepath.Abs("testdata/bin")
	if err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			return setup(env, fakes)
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"eventually": eventually,
			"capture":    capture,
			"repo":       repo,
		},
	})
}

func setup(env *testscript.Env, fakes string) error {
	sep := string(os.PathListSeparator)
	env.Setenv("PATH", binDir+sep+fakes+sep+env.Getenv("PATH"))
	// why: macOS caps Unix socket paths at 104 bytes, so the home that holds agentws.sock must be short.
	tmp, err := os.MkdirTemp("", "aws")
	if err != nil {
		return err
	}
	// why: git and the daemon report resolved paths, and macOS temp dirs sit behind the /var symlink.
	home, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		return err
	}
	work, err := filepath.EvalSymlinks(env.WorkDir)
	if err != nil {
		return err
	}
	socket := fmt.Sprintf("agentws-e2e-%d-%d", os.Getpid(), time.Now().UnixNano())
	for k, v := range map[string]string{
		"AGENTWS_HOME":         home,
		"AGENTWS_E2E":          work,
		"AGENTWS_TMUX_SOCKET":  socket,
		"AGENTWS_TEST_CLOCK":   "+5h",
		"AGENTWS_TEST_PR_POLL": "100ms",
		"GIT_CONFIG_GLOBAL":    "/dev/null",
		"GIT_CONFIG_SYSTEM":    "/dev/null",
		"GIT_AUTHOR_NAME":      "t",
		"GIT_AUTHOR_EMAIL":     "t@example.com",
		"GIT_COMMITTER_NAME":   "t",
		"GIT_COMMITTER_EMAIL":  "t@example.com",
		"GIT_TERMINAL_PROMPT":  "0",
	} {
		env.Setenv(k, v)
	}
	if err := os.MkdirAll(filepath.Join(work, "gates"), 0o755); err != nil {
		return err
	}
	env.Defer(func() {
		stop := exec.Command(filepath.Join(binDir, "agentws"), "daemon", "stop")
		stop.Env = append(os.Environ(), "AGENTWS_HOME="+home)
		_ = stop.Run()
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
		// why: kill-server leaves the socket file behind.
		dir := os.Getenv("TMUX_TMPDIR")
		if dir == "" {
			dir = "/tmp"
		}
		_ = os.Remove(filepath.Join(dir, fmt.Sprintf("tmux-%d", os.Getuid()), socket))
		_ = os.RemoveAll(home)
	})
	return nil
}

// eventually <regexp> <command> [args...] runs the command until its stdout
// matches, failing after waitWithin; `! eventually` runs it until its stdout
// no longer matches. The last run's stdout stays for the commands that follow.
func eventually(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) < 2 {
		ts.Fatalf("usage: [!] eventually <regexp> <command> [args...]")
	}
	re, err := regexp.Compile(`(?m)` + args[0])
	ts.Check(err)
	deadline := time.Now().Add(waitWithin)
	for {
		runErr := ts.Exec(args[1], args[2:]...)
		out := ts.ReadFile("stdout")
		if runErr == nil && re.MatchString(out) != neg {
			return
		}
		if time.Now().After(deadline) {
			want := "no match"
			if neg {
				want = "still a match"
			}
			ts.Fatalf("%s for %q after %v; last stdout:\n%s\nstderr:\n%s\nerr: %v", want, args[0], waitWithin, out, ts.ReadFile("stderr"), runErr)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// capture <var> <regexp> sets var to the regexp's first group in the last
// stdout.
func capture(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: capture <var> <regexp>")
	}
	re, err := regexp.Compile(`(?m)` + args[1])
	ts.Check(err)
	m := re.FindStringSubmatch(ts.ReadFile("stdout"))
	if len(m) < 2 {
		ts.Fatalf("no match for %q in stdout", args[1])
	}
	ts.Setenv(args[0], m[1])
}

// repo <name> makes $WORK/<name>, a clone of a local bare origin whose origin
// URL then points at github.com/o/<name>, so the gh adapter can name it while
// origin/main stays a local ref.
func repo(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: repo <name>")
	}
	name := args[0]
	work := ts.Getenv("AGENTWS_E2E")
	seed := filepath.Join(work, "seed-"+name)
	bare := filepath.Join(work, "origin-"+name+".git")
	dir := filepath.Join(work, name)
	ts.Check(os.MkdirAll(seed, 0o755))
	ts.Check(os.WriteFile(filepath.Join(seed, "README.md"), []byte(name+"\n"), 0o644))
	for _, c := range [][]string{
		{"-C", seed, "init", "-q", "-b", "main"},
		{"-C", seed, "add", "."},
		{"-C", seed, "commit", "-q", "-m", "init"},
		{"clone", "-q", "--bare", seed, bare},
		{"clone", "-q", bare, dir},
		{"-C", dir, "remote", "set-url", "origin", "https://github.com/o/" + name + ".git"},
	} {
		if err := ts.Exec("git", c...); err != nil {
			ts.Fatalf("git %s: %v\n%s", strings.Join(c, " "), err, ts.ReadFile("stderr"))
		}
	}
}
