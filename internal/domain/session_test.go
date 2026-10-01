package domain_test

import (
	"errors"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

func TestNewRandomSessionID(t *testing.T) {
	a, b := domain.NewRandomSessionID(), domain.NewRandomSessionID()
	if a == b {
		t.Fatalf("two IDs are equal: %s", a)
	}
	if _, err := domain.ParseSessionID(string(a)); err != nil {
		t.Errorf("generated ID does not parse: %v", err)
	}
}

func TestParseSessionID(t *testing.T) {
	tests := map[string]bool{
		"0f8fad5b-d9cb-469f-a165-70867728950e": true,
		"0F8FAD5B-D9CB-469F-A165-70867728950E": false,
		"../../etc":                            false,
		"":                                     false,
	}
	for in, ok := range tests {
		if _, err := domain.ParseSessionID(in); (err == nil) != ok {
			t.Errorf("ParseSessionID(%q) error = %v, want ok=%v", in, err, ok)
		}
	}
}

func TestSession_CheckAgent(t *testing.T) {
	s := domain.Session{ID: "id", AgentName: "repo-dev"}
	if err := s.CheckAgent("repo-dev"); err != nil {
		t.Errorf("CheckAgent(same) error = %v", err)
	}
	if err := s.CheckAgent("repo-pm"); !errors.Is(err, domain.ErrSessionAgentMismatch) {
		t.Errorf("CheckAgent(other) error = %v, want ErrSessionAgentMismatch", err)
	}
}
