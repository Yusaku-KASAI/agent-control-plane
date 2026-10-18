package usagelog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/usagelog"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

func TestLog_Record(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acp", "usage.jsonl")
	log := usagelog.New(path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	started := time.Date(2026, 10, 18, 1, 0, 0, 0, time.UTC)
	total := domain.Usage{CostUSD: 1.5, Models: map[string]domain.ModelUsage{"claude-opus": {InputTokens: 10, OutputTokens: 20, CostUSD: 1.5}}}
	turn := domain.Usage{CostUSD: 0.5, Models: map[string]domain.ModelUsage{"claude-opus": {InputTokens: 4, OutputTokens: 5, CostUSD: 0.5}}}

	log.Record(context.Background(), usecase.TurnUsage{
		Agent: "repo-dev", Session: "0f8fad5b-d9cb-469f-a165-70867728950e",
		Conversation: domain.Conversation{Platform: "slack", Workspace: "T1", Channel: "C1", Thread: "1.0"},
		StartedAt:    started, FinishedAt: started.Add(time.Minute),
		Requesters: []domain.Requester{{Platform: "slack", ID: "U_ALICE"}},
		Result:     usecase.TurnResult{Output: "secret answer", Usage: total},
		Usage:      &turn,
	})
	log.Record(context.Background(), usecase.TurnUsage{
		Agent: "repo-dev", Session: "6ba7b810-9dad-41d1-80b4-00c04fd430c8",
		StartedAt: started, FinishedAt: started.Add(time.Minute), AgentStarted: true,
		Result: usecase.TurnResult{Error: true, UsageLimited: true, Usage: total},
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("secret answer")) {
		t.Error("the turn's output was recorded")
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	want := []string{
		`{"finished_at":"2026-10-18T01:01:00Z","started_at":"2026-10-18T01:00:00Z","agent":"repo-dev","session":"0f8fad5b-d9cb-469f-a165-70867728950e",` +
			`"conversation":{"platform":"slack","workspace":"T1","channel":"C1","thread":"1.0"},"requesters":[{"platform":"slack","id":"U_ALICE"}],"agent_started":false,"outcome":"succeeded",` +
			`"usage":{"cost_usd":0.5,"models":{"claude-opus":{"input_tokens":4,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"cost_usd":0.5}}},` +
			`"reported_total":{"cost_usd":1.5,"models":{"claude-opus":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"cost_usd":1.5}}}}`,
		`{"finished_at":"2026-10-18T01:01:00Z","started_at":"2026-10-18T01:00:00Z","agent":"repo-dev","session":"6ba7b810-9dad-41d1-80b4-00c04fd430c8",` +
			`"requesters":[],"agent_started":true,"outcome":"usage_limited","usage":null,` +
			`"reported_total":{"cost_usd":1.5,"models":{"claude-opus":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"cost_usd":1.5}}}}`,
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(want), data)
	}
	for i := range want {
		if !json.Valid([]byte(lines[i])) || lines[i] != want[i] {
			t.Errorf("line %d =\n%s\nwant\n%s", i, lines[i], want[i])
		}
	}
}

func TestLog_RecordFailureIsLogged(t *testing.T) {
	dir := t.TempDir()
	var logged bytes.Buffer
	// A directory where the file should be makes the append fail.
	path := filepath.Join(dir, "usage.jsonl")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	log := usagelog.New(path, slog.New(slog.NewTextHandler(&logged, nil)))
	log.Record(context.Background(), usecase.TurnUsage{Agent: "repo-dev"})
	if !strings.Contains(logged.String(), "recording a turn's usage failed") {
		t.Errorf("log = %q, want the failure", logged.String())
	}
}
