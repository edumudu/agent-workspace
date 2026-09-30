package procs

import (
	"reflect"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestPortsParseListenAndCwd(t *testing.T) {
	listen := "p101\ng100\ncnode\nf23\nn*:8081\nf24\nn[::1]:8081\nf25\nn127.0.0.1:9229\n" +
		"p202\ng202\ncControlCenter helper\nf9\nn*:7000\n" +
		"p303\ng300\ncbun\nf11\nn[fe80::1%lo0]:3000\n"
	cwd := "p101\nfcwd\nn/w/api-feat\np202\nfcwd\nn/\n"

	got := merge(parseListen(listen), parseCwd(cwd))

	want := []domain.Listener{
		{Port: 8081, PID: 101, PGID: 100, Command: "node", Cwd: "/w/api-feat"},
		{Port: 8081, PID: 101, PGID: 100, Command: "node", Cwd: "/w/api-feat"},
		{Port: 9229, PID: 101, PGID: 100, Command: "node", Cwd: "/w/api-feat"},
		{Port: 7000, PID: 202, PGID: 202, Command: "ControlCenter helper", Cwd: "/"},
		{Port: 3000, PID: 303, PGID: 300, Command: "bun"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestPortsParseSkipsMalformedLines(t *testing.T) {
	listen := "pabc\ncbad\nn*:1\np5\ng5\ncok\nn*:notaport\nn*:80\nn\nf3\n"
	got := parseListen(listen)
	want := []domain.Listener{{Port: 80, PID: 5, PGID: 5, Command: "ok"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPortsParseEmpty(t *testing.T) {
	if got := parseListen(""); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
	if got := parseCwd(""); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestPortsPIDsOfDeduplicatesAndSorts(t *testing.T) {
	got := pidList([]domain.Listener{{PID: 9}, {PID: 3}, {PID: 9}, {PID: 5}})
	if want := "3,5,9"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
