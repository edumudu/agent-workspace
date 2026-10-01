package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestVersionTellsAboutNewerRelease(t *testing.T) {
	var out bytes.Buffer
	latest := func(context.Context) (string, error) { return "v0.1.0-alpha.3", nil }
	if code := runVersion("v0.1.0-alpha.2", "abc", latest, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := out.String()
	for _, want := range []string{"agentws v0.1.0-alpha.2 (commit abc)", "v0.1.0-alpha.3 is available", "agentws daemon stop"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q lacks %q", got, want)
		}
	}
}

func TestVersionQuietWhenCurrentOrCheckFails(t *testing.T) {
	for _, latest := range []func(context.Context) (string, error){
		func(context.Context) (string, error) { return "v0.1.0-alpha.2", nil },
		func(context.Context) (string, error) { return "", errors.New("offline") },
	} {
		var out bytes.Buffer
		runVersion("v0.1.0-alpha.2", "abc", latest, &out)
		if got := out.String(); got != "agentws v0.1.0-alpha.2 (commit abc)\n" {
			t.Errorf("output = %q", got)
		}
	}
}
