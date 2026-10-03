package slackbot

import (
	"testing"
	"time"
)

func TestSeenMessages(t *testing.T) {
	s := newSeenMessages(time.Minute)
	now := time.Unix(1_700_000_000, 0)

	if !s.first("C1/1.0", now) {
		t.Fatal("first delivery was treated as a repeat")
	}
	if s.first("C1/1.0", now.Add(10*time.Second)) {
		t.Error("redelivery was treated as new")
	}
	if !s.first("C1/2.0", now.Add(10*time.Second)) {
		t.Error("another message was treated as a repeat")
	}
	if !s.first("C1/1.0", now.Add(2*time.Minute)) {
		t.Error("the message was remembered past the TTL")
	}
}
