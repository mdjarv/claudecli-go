//go:build integration

package claudecli

import (
	"context"
	"testing"
	"time"
)

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
