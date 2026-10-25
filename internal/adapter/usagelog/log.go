// Package usagelog keeps what each turn spent as a line of JSON in one file.
package usagelog

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// Log appends to the file at its path, opening it for each line so that the file can be moved
// away or removed while the control plane runs.
type Log struct {
	path string
	log  *slog.Logger
	mu   sync.Mutex
}

func New(path string, log *slog.Logger) *Log {
	return &Log{path: path, log: log}
}

type line struct {
	FinishedAt   time.Time     `json:"finished_at"`
	StartedAt    time.Time     `json:"started_at"`
	Agent        string        `json:"agent"`
	Session      string        `json:"session"`
	Conversation *conversation `json:"conversation,omitempty"`
	Requesters   []requester   `json:"requesters"`
	AgentStarted bool          `json:"agent_started"`
	Outcome      string        `json:"outcome"`
	// Usage is null when the turn's usage cannot be told.
	Usage         *usage `json:"usage"`
	ReportedTotal usage  `json:"reported_total"`
	// ContextTokens is null when the turn made no API request that reported it.
	ContextTokens *int64 `json:"context_tokens"`
}

type conversation struct {
	Platform  string `json:"platform"`
	Workspace string `json:"workspace"`
	Channel   string `json:"channel"`
	Thread    string `json:"thread"`
}

type requester struct {
	Platform string `json:"platform"`
	ID       string `json:"id"`
}

type usage struct {
	CostUSD float64               `json:"cost_usd"`
	Models  map[string]modelUsage `json:"models"`
}

type modelUsage struct {
	InputTokens              int64   `json:"input_tokens"`
	OutputTokens             int64   `json:"output_tokens"`
	CacheReadInputTokens     int64   `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64   `json:"cache_creation_input_tokens"`
	CostUSD                  float64 `json:"cost_usd"`
}

func fromUsage(u domain.Usage) usage {
	r := usage{CostUSD: u.CostUSD, Models: make(map[string]modelUsage, len(u.Models))}
	for name, m := range u.Models {
		r.Models[name] = modelUsage(m)
	}
	return r
}

func outcome(r usecase.TurnResult) string {
	switch {
	case r.Interrupted:
		return "interrupted"
	case r.UsageLimited:
		return "usage_limited"
	case r.Error:
		return "error"
	default:
		return "succeeded"
	}
}

func newLine(u usecase.TurnRecord) line {
	l := line{
		FinishedAt:    u.FinishedAt.UTC(),
		StartedAt:     u.StartedAt.UTC(),
		Agent:         string(u.Agent),
		Session:       string(u.Session),
		Requesters:    make([]requester, 0, len(u.Requesters)),
		AgentStarted:  u.AgentStarted,
		Outcome:       outcome(u.Result),
		ReportedTotal: fromUsage(u.Result.Usage),
	}
	if c := u.Conversation; c != (domain.Conversation{}) {
		l.Conversation = &conversation{Platform: c.Platform, Workspace: c.Workspace, Channel: c.Channel, Thread: c.Thread}
	}
	for _, r := range u.Requesters {
		l.Requesters = append(l.Requesters, requester{Platform: r.Platform, ID: r.ID})
	}
	if u.Usage != nil {
		turn := fromUsage(*u.Usage)
		l.Usage = &turn
	}
	if n := u.Result.ContextTokens; n > 0 {
		l.ContextTokens = &n
	}
	return l
}

func (l *Log) Record(_ context.Context, u usecase.TurnRecord) {
	if err := l.append(newLine(u)); err != nil {
		l.log.Error("recording a turn's usage failed", "agent", u.Agent, "session", u.Session, "err", err)
	}
}

func (l *Log) append(v line) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return errors.Join(err, f.Close())
}
