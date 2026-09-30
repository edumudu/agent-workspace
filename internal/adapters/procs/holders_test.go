package procs

import (
	"reflect"
	"testing"
)

func TestCleanupHoldersFromLsofOutput(t *testing.T) {
	out := "p10\nczsh\nfcwd\nn/w/api-a\np11\ncnvim\nfcwd\nn/w/api-a/src\np12\ncnode\nfcwd\nn/w/api-ab\np13\ncsleep\nfcwd\nn/w/web\n"
	got := holders([]byte(out), []string{"/w/api-a", "/w/api-ab/", "/w/other"}, map[string]string{})
	want := map[string][]string{
		"/w/api-a":   {"zsh (pid 10)", "nvim (pid 11)"},
		"/w/api-ab/": {"node (pid 12)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("holders = %v, want %v", got, want)
	}
}

func TestCleanupHoldersMatchTheResolvedPathToo(t *testing.T) {
	out := "p10\nczsh\nfcwd\nn/private/var/t/wt\n"
	got := holders([]byte(out), []string{"/var/t/wt"}, map[string]string{"/var/t/wt": "/private/var/t/wt"})
	if len(got["/var/t/wt"]) != 1 {
		t.Errorf("holders = %v, want zsh through the resolved path", got)
	}
}
