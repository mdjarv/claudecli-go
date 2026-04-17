//go:build integration

package claudecli

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"testing"
	"time"
)

// newTestSessionID returns a random UUIDv4 string for integration tests that
// need a persisted session (RunBlocking adds --no-session-persistence unless a
// session flag is set).
func newTestSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func TestIntegrationRealCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := New(WithModel(ModelHaiku))
	text, result, err := client.RunText(ctx, "Say hello in exactly 3 words")
	if err != nil {
		t.Fatalf("RunText: %v", err)
	}
	if text == "" {
		t.Error("empty text response")
	}
	if result == nil {
		t.Fatal("nil result")
	}
	if result.CostUSD <= 0 {
		t.Error("zero cost")
	}
	if result.SessionID == "" {
		t.Error("missing session ID")
	}
	t.Logf("response: %q", text)
	t.Logf("cost: $%.6f, tokens: %d/%d", result.CostUSD, result.Usage.InputTokens, result.Usage.OutputTokens)
}

func TestIntegrationStreaming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := New(WithModel(ModelHaiku))
	stream := client.Run(ctx, "Count from 1 to 3")

	var eventTypes []string
	for event := range stream.Events() {
		switch event.(type) {
		case *StartEvent:
			eventTypes = append(eventTypes, "start")
		case *InitEvent:
			eventTypes = append(eventTypes, "init")
		case *TextEvent:
			eventTypes = append(eventTypes, "text")
		case *ResultEvent:
			eventTypes = append(eventTypes, "result")
		}
	}

	if stream.State() != StateDone {
		t.Errorf("expected StateDone, got %s", stream.State())
	}

	for _, required := range []string{"start", "init", "text", "result"} {
		found := false
		for _, et := range eventTypes {
			if et == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %s event", required)
		}
	}
}

// TestIntegrationForkResume verifies that WithResume + WithForkSession plumbs
// through to the CLI correctly: the fork reuses the parent's conversation
// context (parent's marker appears in the reply) while running in a distinct
// session ID (parent file on disk untouched). Guards the option.go fix that
// emits --fork-session alongside --resume.
func TestIntegrationForkResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client := New(WithModel(ModelHaiku))

	parentSID := newTestSessionID()
	parent, err := client.RunBlocking(ctx,
		"Remember the marker 'zebra17'. Reply with just the marker.",
		WithSessionID(parentSID),
	)
	if err != nil {
		t.Fatalf("parent RunBlocking: %v", err)
	}
	if parent.SessionID == "" {
		t.Fatal("parent session ID missing")
	}
	t.Logf("parent session: %s", parent.SessionID)

	fork, err := client.RunBlocking(ctx,
		"What marker did I ask you to remember? Reply with just the marker.",
		WithResume(parent.SessionID),
		WithForkSession(),
	)
	if err != nil {
		t.Fatalf("fork RunBlocking: %v", err)
	}
	if fork.SessionID == "" {
		t.Error("fork session ID missing")
	}
	if fork.SessionID == parent.SessionID {
		t.Errorf("fork reused parent session ID %q — --fork-session not applied", parent.SessionID)
	}
	if !strings.Contains(fork.Text, "zebra17") {
		t.Errorf("fork reply missing marker; got %q", fork.Text)
	}
	t.Logf("fork session: %s  reply=%q  cost=$%.6f  cache_read=%d",
		fork.SessionID, fork.Text, fork.CostUSD, fork.Usage.CacheReadTokens)
}
