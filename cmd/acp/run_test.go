package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	}{
		{"answered", answer(http.StatusOK, httpapi.TurnResponse{Output: "done"}), 0},
		{"interrupted", answer(http.StatusOK, httpapi.TurnResponse{Interrupted: true}), 0},
		{"ended on an error", answer(http.StatusOK, httpapi.TurnResponse{Output: "API Error", Error: true}), 1},
		{"stopped at the usage limit", answer(http.StatusOK, httpapi.TurnResponse{Error: true, UsageLimited: true}), 1},
		{"refused", answer(http.StatusTooManyRequests, httpapi.ErrorResponse{Code: httpapi.CodeInboxFull, Message: "full"}), 2},
		{"failed in the server", answer(http.StatusInternalServerError, httpapi.ErrorResponse{Code: httpapi.CodeInternal, Message: "boom"}), 1},
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
}
