package domain_test

import (
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

func TestNewAgent(t *testing.T) {
	valid := domain.Target{Path: "/src/repo", Image: "repo-dev"}

	tests := []struct {
		name    string
		agent   domain.AgentName
		role    domain.Role
		target  domain.Target
		wantErr bool
	}{
		{name: "valid", agent: "repo-dev", role: "developer", target: valid},
		{name: "empty name", role: "developer", target: valid, wantErr: true},
		{name: "empty role", agent: "repo-dev", target: valid, wantErr: true},
		{name: "relative path", agent: "repo-dev", role: "developer", target: domain.Target{Path: "repo", Image: "repo-dev"}, wantErr: true},
		{name: "empty image", agent: "repo-dev", role: "developer", target: domain.Target{Path: "/src/repo"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewAgent(tt.agent, tt.role, tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAgent() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAgent() error = %v", err)
			}
			want := domain.Agent{Name: tt.agent, Role: tt.role, Target: tt.target}
			if got != want {
				t.Errorf("NewAgent() = %+v, want %+v", got, want)
			}
		})
	}
}
