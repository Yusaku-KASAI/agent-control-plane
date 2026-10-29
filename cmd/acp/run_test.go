package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/httpapi"
)

// The exit codes of acp run, as docs/reference/usage.md tells them.
func TestRun_ExitCodes(t *testing.T) {
	answer := func(status int, v any) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(v)
		}
	}
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    int
		// wantStderr is a part of what acp run writes to stderr, if any.
		wantStderr string
	}{
		{"answered", answer(http.StatusOK, httpapi.TurnResponse{Output: "done"}), 0, ""},
		{"interrupted", answer(http.StatusOK, httpapi.TurnResponse{Interrupted: true}), 0, "interrupted"},
		{"ended on an error", answer(http.StatusOK, httpapi.TurnResponse{Output: "API Error", Error: true}), 1, "ended on an error"},
		{"stopped at the usage limit", answer(http.StatusOK, httpapi.TurnResponse{Error: true, UsageLimited: true}), 1, "usage limit"},
		{"refused", answer(http.StatusConflict, httpapi.ErrorResponse{Code: httpapi.CodeSessionBusy, Message: "busy"}), 2, ""},
		{"failed in the server", answer(http.StatusInternalServerError, httpapi.ErrorResponse{Code: httpapi.CodeInternal, Message: "boom"}), 1, ""},
		{"shutting down", answer(http.StatusServiceUnavailable, httpapi.ErrorResponse{Code: httpapi.CodeUnavailable, Message: "restart"}), 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			env := map[string]string{"ACP_SERVER": srv.URL, "ACP_API_TOKEN": "token"}
			var stdout, stderr bytes.Buffer
			if code := dispatch(context.Background(), []string{"run", "--agent", "a", "hello"}, func(k string) string { return env[k] }, &stdout, &stderr); code != tt.want {
				t.Errorf("acp run = %d, want %d (stderr: %s)", code, tt.want, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}

	t.Run("server unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		env := map[string]string{"ACP_SERVER": srv.URL, "ACP_API_TOKEN": "token"}
		var stdout, stderr bytes.Buffer
		if code := dispatch(context.Background(), []string{"run", "--agent", "a", "hello"}, func(k string) string { return env[k] }, &stdout, &stderr); code != 1 {
			t.Errorf("acp run = %d, want 1 (stderr: %s)", code, stderr.String())
		}
	})

	t.Run("flag error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := dispatch(context.Background(), []string{"run", "hello"}, func(string) string { return "" }, &stdout, &stderr); code != 2 {
			t.Errorf("acp run without --agent = %d, want 2", code)
		}
	})

	t.Run("stopped by a signal", func(t *testing.T) {
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
		defer srv.Close()
		defer close(release)
		env := map[string]string{"ACP_SERVER": srv.URL, "ACP_API_TOKEN": "token"}
		// The signal handler of main cancels the context.
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)
		var stdout, stderr bytes.Buffer
		if code := dispatch(ctx, []string{"run", "--agent", "a", "hello"}, func(k string) string { return env[k] }, &stdout, &stderr); code != 1 {
			t.Errorf("acp run = %d, want 1 (stderr: %s)", code, stderr.String())
		}
	})
}

// The exit codes of acp stop, as docs/reference/usage.md tells them.
func TestStop_ExitCodes(t *testing.T) {
	answer := func(status int, v any) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if v != nil {
				_ = json.NewEncoder(w).Encode(v)
			}
		}
	}
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    int
	}{
		{"stopped", answer(http.StatusNoContent, nil), 0},
		{"refused", answer(http.StatusNotFound, httpapi.ErrorResponse{Code: httpapi.CodeSessionNotFound, Message: "no session"}), 2},
		{"failed in the server", answer(http.StatusInternalServerError, httpapi.ErrorResponse{Code: httpapi.CodeInternal, Message: "boom"}), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			env := map[string]string{"ACP_SERVER": srv.URL, "ACP_API_TOKEN": "token"}
			var stdout, stderr bytes.Buffer
			if code := dispatch(context.Background(), []string{"stop", "--agent", "a", "--session", "s"}, func(k string) string { return env[k] }, &stdout, &stderr); code != tt.want {
				t.Errorf("acp stop = %d, want %d (stderr: %s)", code, tt.want, stderr.String())
			}
		})
	}
}
