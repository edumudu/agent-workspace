package github

import (
	"reflect"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestWorktreeDetectParsePRs(t *testing.T) {
	out := `[
	  {"number":12,"title":"Add login","url":"https://github.com/o/api/pull/12","headRefName":"feat","state":"OPEN",
	   "statusCheckRollup":[
	     {"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"},
	     {"__typename":"CheckRun","status":"IN_PROGRESS","conclusion":""},
	     {"__typename":"StatusContext","state":"SUCCESS"}
	   ]},
	  {"number":11,"title":"Fix","url":"u11","headRefName":"fix","state":"MERGED",
	   "statusCheckRollup":[
	     {"__typename":"CheckRun","status":"COMPLETED","conclusion":"SKIPPED"},
	     {"__typename":"StatusContext","state":"ERROR"}
	   ]},
	  {"number":10,"title":"Docs","url":"u10","headRefName":"docs","state":"CLOSED","statusCheckRollup":[
	     {"__typename":"CheckRun","status":"COMPLETED","conclusion":"NEUTRAL"},
	     {"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"}
	  ]},
	  {"number":9,"title":"None","url":"u9","headRefName":"none","state":"OPEN","statusCheckRollup":[]},
	  {"number":8,"title":"Pending","url":"u8","headRefName":"p","state":"OPEN","statusCheckRollup":[
	     {"__typename":"StatusContext","state":"PENDING"},
	     {"__typename":"CheckRun","status":"COMPLETED","conclusion":"TIMED_OUT"}
	  ]}
	]`
	got, err := parsePRs([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.PullRequest{
		{Number: 12, Title: "Add login", URL: "https://github.com/o/api/pull/12", Head: "feat", State: domain.PROpen, Checks: domain.CheckPending},
		{Number: 11, Title: "Fix", URL: "u11", Head: "fix", State: domain.PRMerged, Checks: domain.CheckFailing},
		{Number: 10, Title: "Docs", URL: "u10", Head: "docs", State: domain.PRClosed, Checks: domain.CheckPassing},
		{Number: 9, Title: "None", URL: "u9", Head: "none", State: domain.PROpen, Checks: domain.CheckNone},
		{Number: 8, Title: "Pending", URL: "u8", Head: "p", State: domain.PROpen, Checks: domain.CheckFailing},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestWorktreeDetectParsePRsRejectsGarbage(t *testing.T) {
	if _, err := parsePRs([]byte("not json")); err == nil {
		t.Fatal("want an error")
	}
}
