package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRun_UsageErrors(t *testing.T) {
	tests := map[string][]string{
		"no subcommand":      nil,
		"unknown subcommand": {"serve"},
		"missing agent":      {"run", "hello"},
		"unknown flag":       {"run", "--nope"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), args, &stdout, &stderr); code != 2 {
				t.Errorf("run(%q) = %d, want 2", args, code)
			}
		})
	}
}
