package domain

import (
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrSessionNotFound      = errors.New("session not found")
	ErrSessionBusy          = errors.New("session is in use")
	ErrSessionAgentMismatch = errors.New("session belongs to another agent")
)

// SessionID is a UUID so that it can double as the Claude Code session ID.
type SessionID string

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ParseSessionID(s string) (SessionID, error) {
	if !uuidPattern.MatchString(s) {
		return "", fmt.Errorf("session id %q is not a lowercase UUID", s)
	}
	return SessionID(s), nil
}

// Branch is the session's default working branch.
func (id SessionID) Branch() string {
	return "acp/" + string(id)[:8]
}

// OwnsBranch tells whether the session created the branch: its default working branch, or one
// named after it with a "-" suffix.
func (id SessionID) OwnsBranch(name string) bool {
	return BranchOwnedBy(id.Branch(), name)
}

// BranchOwnedBy tells whether a session whose default working branch is branch created the
// branch name.
func BranchOwnedBy(branch, name string) bool {
	return name == branch || strings.HasPrefix(name, branch+"-")
}

func NewRandomSessionID() SessionID {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // RFC 4122 variant
	return SessionID(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
}

type Session struct {
	ID        SessionID
	AgentName AgentName
	Workspace Workspace
	// Conversation is where the session started; zero when it did not start in a chat.
	Conversation Conversation
	// Turns counts completed turns. A session with any resumes its Claude Code conversation.
	Turns int
	// LastUsedAt is when the session was created, last took a request, last began or ended a turn, or
	// last had a background task end: the last time its work may have changed.
	LastUsedAt time.Time
	// KeptNotifiedAt is when the thread was last told that the session is kept for the work it holds.
	KeptNotifiedAt time.Time
	// ReportedUsage is the session's total that Claude Code reported at the end of the last turn; nil
	// when it is unknown, as after a process that ended by itself, which may not have saved it.
	ReportedUsage *Usage
}

// UsageBefore is the session's total to compare the next turn's with: nothing was spent before the
// first turn.
func (s Session) UsageBefore() *Usage {
	if s.ReportedUsage == nil && s.IsNew() {
		return &Usage{}
	}
	return s.ReportedUsage
}

// IsNew tells whether the session has finished no turn. Its Claude Code conversation may exist all the
// same, left by a first turn that ended before finishing.
func (s Session) IsNew() bool {
	return s.Turns == 0
}

func (s Session) CheckAgent(name AgentName) error {
	if s.AgentName != name {
		return fmt.Errorf("%w: session %s is for %q, not %q", ErrSessionAgentMismatch, s.ID, s.AgentName, name)
	}
	return nil
}
