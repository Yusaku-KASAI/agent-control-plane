package domain

// Usage is what Claude Code reports as spent: its estimate of the cost, and the tokens and cost per
// model, subagents included.
type Usage struct {
	CostUSD float64
	Models  map[string]ModelUsage
}

type ModelUsage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
	CostUSD                  float64
}

func (u Usage) IsZero() bool {
	return u.CostUSD == 0 && len(u.Models) == 0
}

// since returns what u spent after earlier, both running totals of the same count. It is false when
// the count started over in between: when any figure of earlier is larger.
func (u Usage) since(earlier Usage) (Usage, bool) {
	if u.CostUSD < earlier.CostUSD {
		return Usage{}, false
	}
	d := Usage{CostUSD: u.CostUSD - earlier.CostUSD}
	for name, e := range earlier.Models {
		if _, ok := u.Models[name]; !ok && e != (ModelUsage{}) {
			return Usage{}, false
		}
	}
	for name, m := range u.Models {
		e := earlier.Models[name]
		if m.InputTokens < e.InputTokens || m.OutputTokens < e.OutputTokens ||
			m.CacheReadInputTokens < e.CacheReadInputTokens || m.CacheCreationInputTokens < e.CacheCreationInputTokens ||
			m.CostUSD < e.CostUSD {
			return Usage{}, false
		}
		diff := ModelUsage{
			InputTokens:              m.InputTokens - e.InputTokens,
			OutputTokens:             m.OutputTokens - e.OutputTokens,
			CacheReadInputTokens:     m.CacheReadInputTokens - e.CacheReadInputTokens,
			CacheCreationInputTokens: m.CacheCreationInputTokens - e.CacheCreationInputTokens,
			CostUSD:                  m.CostUSD - e.CostUSD,
		}
		if diff != (ModelUsage{}) {
			if d.Models == nil {
				d.Models = make(map[string]ModelUsage)
			}
			d.Models[name] = diff
		}
	}
	return d, true
}

// TurnUsage tells what a turn spent from the session's total that Claude Code reported at the end
// of the turn. known is the session's total after the previous turn, nil when unknown. It returns
// the turn's usage, nil when it cannot be told, and the total to compare the next turn with.
//
// Claude Code reports a running total that a resumed session carries over. The total starts over
// on /clear; the turn's usage is then the reported total, which counts from the restart. A crashed
// turn may report zeros, which say nothing about the total.
func TurnUsage(known *Usage, reported Usage) (turn, next *Usage) {
	switch {
	case known == nil:
		return nil, &reported
	case reported.IsZero() && !known.IsZero():
		return nil, known
	}
	if d, ok := reported.since(*known); ok {
		return &d, &reported
	}
	return &reported, &reported
}
