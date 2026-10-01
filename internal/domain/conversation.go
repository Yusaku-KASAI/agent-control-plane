package domain

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"strings"
)

var ErrRequesterNotAllowed = errors.New("requester is not allowed to use this agent")

// Conversation identifies a thread on a chat platform, independent of how its messages arrive.
type Conversation struct {
	Platform  string
	Workspace string
	Channel   string
	Thread    string
}

type Requester struct {
	Platform string
	ID       string
}

// conversationNamespace is the UUIDv5 namespace for sessions derived from conversations.
var conversationNamespace = [16]byte{0x6f, 0x3a, 0x1c, 0x52, 0x0b, 0x8e, 0x4d, 0x27, 0x9a, 0x41, 0x55, 0xc3, 0x2e, 0x07, 0xd9, 0x8b}

// SessionID is stable for the same agent and thread, so every mention in a thread continues one session.
func (c Conversation) SessionID(agent AgentName) SessionID {
	name := strings.Join([]string{string(agent), c.Platform, c.Workspace, c.Channel, c.Thread}, "\x00")
	h := sha1.New()
	h.Write(conversationNamespace[:])
	h.Write([]byte(name))
	b := h.Sum(nil)[:16]
	b[6] = b[6]&0x0f | 0x50 // version 5
	b[8] = b[8]&0x3f | 0x80 // RFC 4122 variant
	return SessionID(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
}
