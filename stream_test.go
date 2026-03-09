package claudecli

import (
	"context"
	"testing"
)

func TestStreamState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 10)
	done := make(chan struct{})
	s := newStream(ctx, events, done, cancel)

	if s.State() != StateStarting {
		t.Fatalf("expected StateStarting, got %s", s.State())
	}

	events <- &InitEvent{SessionID: "test"}
	e, ok := s.Next()
	if !ok {
		t.Fatal("expected event")
	}
	if _, ok := e.(*InitEvent); !ok {
		t.Fatalf("expected InitEvent, got %T", e)
	}
	if s.State() != StateRunning {
		t.Fatalf("expected StateRunning, got %s", s.State())
	}

	result := &ResultEvent{Text: "hello", CostUSD: 0.01}
	events <- result
	e, ok = s.Next()
	if !ok {
		t.Fatal("expected event")
	}
	if _, ok := e.(*ResultEvent); !ok {
		t.Fatalf("expected ResultEvent, got %T", e)
	}
	if s.State() != StateDone {
		t.Fatalf("expected StateDone, got %s", s.State())
	}

	close(events)
	close(done)

	// Wait should be idempotent and return cached result
	r1, err1 := s.Wait()
	r2, err2 := s.Wait()
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error: %v, %v", err1, err2)
	}
	if r1 != r2 {
		t.Error("Wait() not idempotent")
	}
	if r1.Text != "hello" {
		t.Errorf("expected 'hello', got %q", r1.Text)
	}
}

func TestStreamConcurrentWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 10)
	done := make(chan struct{})
	s := newStream(ctx, events, done, cancel)

	events <- &InitEvent{SessionID: "test"}
	events <- &ResultEvent{Text: "concurrent", CostUSD: 0.01}
	close(events)
	close(done)

	const goroutines = 10
	results := make(chan *ResultEvent, goroutines)
	errs := make(chan error, goroutines)

	for range goroutines {
		go func() {
			r, err := s.Wait()
			results <- r
			errs <- err
		}()
	}

	var first *ResultEvent
	for range goroutines {
		r := <-results
		err := <-errs
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if first == nil {
			first = r
		} else if r != first {
			t.Error("concurrent Wait() returned different pointers")
		}
	}
}
