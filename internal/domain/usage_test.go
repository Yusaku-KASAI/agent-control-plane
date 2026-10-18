package domain_test

import (
	"reflect"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/domain"
)

func TestTurnUsage(t *testing.T) {
	opus := func(in, out int64, cost float64) map[string]domain.ModelUsage {
		return map[string]domain.ModelUsage{"opus": {InputTokens: in, OutputTokens: out, CostUSD: cost}}
	}
	first := domain.Usage{CostUSD: 1, Models: opus(10, 5, 1)}
	later := domain.Usage{CostUSD: 3, Models: map[string]domain.ModelUsage{
		"opus":  {InputTokens: 30, OutputTokens: 5, CostUSD: 2.5},
		"haiku": {InputTokens: 4, CostUSD: 0.5},
	}}
	tests := []struct {
		name     string
		known    *domain.Usage
		reported domain.Usage
		turn     *domain.Usage
		next     *domain.Usage
	}{
		{"first turn", &domain.Usage{}, first, &first, &first},
		{"later turn", &first, later, &domain.Usage{CostUSD: 2, Models: map[string]domain.ModelUsage{
			"opus":  {InputTokens: 20, CostUSD: 1.5},
			"haiku": {InputTokens: 4, CostUSD: 0.5},
		}}, &later},
		{"nothing spent", &first, first, &domain.Usage{}, &first},
		{"unknown total", nil, later, nil, &later},
		{"count started over", &later, first, &first, &first},
		{"model gone", &later, domain.Usage{CostUSD: 4, Models: opus(40, 6, 4)}, &domain.Usage{CostUSD: 4, Models: opus(40, 6, 4)}, &domain.Usage{CostUSD: 4, Models: opus(40, 6, 4)}},
		{"zeros after a crash", &first, domain.Usage{}, nil, &first},
		{"zeros while unknown", nil, domain.Usage{}, nil, nil},
		{"zeros from the start", &domain.Usage{}, domain.Usage{}, &domain.Usage{}, &domain.Usage{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			turn, next := domain.TurnUsage(tc.known, tc.reported)
			if !reflect.DeepEqual(turn, tc.turn) || !reflect.DeepEqual(next, tc.next) {
				t.Errorf("TurnUsage() = %+v, %+v; want %+v, %+v", turn, next, tc.turn, tc.next)
			}
		})
	}
}

func TestSession_UsageBefore(t *testing.T) {
	known := &domain.Usage{CostUSD: 1}
	tests := []struct {
		name string
		s    domain.Session
		want *domain.Usage
	}{
		{"new", domain.Session{}, &domain.Usage{}},
		{"known", domain.Session{Turns: 2, ReportedUsage: known}, known},
		{"unknown", domain.Session{Turns: 2}, nil},
	}
	for _, tc := range tests {
		if got := tc.s.UsageBefore(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: UsageBefore() = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
