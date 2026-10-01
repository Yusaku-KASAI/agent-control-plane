package main

import (
	"bytes"
	"context"
	"testing"
)

func TestDispatch_UsageErrors(t *testing.T) {
	tests := map[string][]string{
		"no subcommand":       nil,
		"unknown subcommand":  {"deploy"},
		"run without agent":   {"run", "hello"},
		"run unknown flag":    {"run", "--nope"},
		"serve without token": {"serve"},
	}
	noEnv := func(string) string { return "" }
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := dispatch(context.Background(), args, noEnv, &stdout, &stderr); code != 2 {
				t.Errorf("dispatch(%q) = %d, want 2 (stderr: %s)", args, code, stderr.String())
			}
		})
	}
}
