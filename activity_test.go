package claudecli

import (
	"context"
	"strings"
	"testing"
)

func TestActivityTrackerIdleToThinking(t *testing.T) {
	tr := newActivityTracker()
	if s := tr.State(); s != ActivityIdle {
		t.Fatalf("initial state = %s, want idle", s)
	}
	trans := tr.observe(&TextEvent{Content: "hi"})
	if trans == nil || trans.State != ActivityThinking {
		t.Fatalf("got %+v, want thinking transition", trans)
	}
	// Subsequent text events do not emit another transition.
	if trans := tr.observe(&TextEvent{Content: "world"}); trans != nil {
		t.Errorf("duplicate thinking transition: %+v", trans)
	}
}

func TestActivityTrackerMarkQueryOnlyWhenIdle(t *testing.T) {
	tr := newActivityTracker()
	if tr.markQuery() == nil {
		t.Fatal("markQuery from idle should emit transition")
	}
	if tr.State() != ActivityThinking {
		t.Errorf("state after markQuery = %s, want thinking", tr.State())
	}
	if tr.markQuery() != nil {
		t.Error("markQuery from thinking should be a no-op")
	}
}

func TestActivityTrackerToolUsePairing(t *testing.T) {
	tr := newActivityTracker()
	// idle → thinking (via text) → awaiting_tool_result (via tool_use)
	if trans := tr.observe(&TextEvent{}); trans == nil || trans.State != ActivityThinking {
		t.Fatalf("text didn't transition to thinking: %+v", trans)
	}
	if trans := tr.observe(&ToolUseEvent{ID: "t1"}); trans == nil || trans.State != ActivityAwaitingToolResult {
		t.Fatalf("tool_use didn't transition to awaiting: %+v", trans)
	}
	// Second concurrent tool_use: no new transition.
	if trans := tr.observe(&ToolUseEvent{ID: "t2"}); trans != nil {
		t.Errorf("duplicate awaiting transition: %+v", trans)
	}
	// First tool_result: still awaiting (1 outstanding).
	if trans := tr.observe(&ToolResultEvent{ToolUseID: "t1"}); trans != nil {
		t.Errorf("premature transition back to thinking: %+v", trans)
	}
	// Second tool_result: back to thinking.
	if trans := tr.observe(&ToolResultEvent{ToolUseID: "t2"}); trans == nil || trans.State != ActivityThinking {
		t.Errorf("final tool_result didn't transition to thinking: %+v", trans)
	}
}

func TestActivityTrackerUserEventToolResult(t *testing.T) {
	// Real CLI flow: tool_use via assistant, tool_result via user event.
	tr := newActivityTracker()
	tr.observe(&TextEvent{})
	tr.observe(&ToolUseEvent{ID: "t1"})
	if tr.State() != ActivityAwaitingToolResult {
		t.Fatalf("setup: state = %s", tr.State())
	}
	trans := tr.observe(&UserEvent{
		Content: []UserContent{{Type: "tool_result", ToolUseID: "t1"}},
	})
	if trans == nil || trans.State != ActivityThinking {
		t.Errorf("UserEvent tool_result didn't transition to thinking: %+v", trans)
	}
}

func TestActivityTrackerSubagentIgnored(t *testing.T) {
	tr := newActivityTracker()
	tr.observe(&TextEvent{})
	// Tool_use with parent = subagent activity; must not change state.
	before := tr.State()
	if trans := tr.observe(&ToolUseEvent{ID: "sub", ParentToolUseID: "parent"}); trans != nil {
		t.Errorf("subagent tool_use emitted transition: %+v", trans)
	}
	if tr.State() != before {
		t.Errorf("subagent event changed state: %s → %s", before, tr.State())
	}
}

func TestActivityTrackerResultResets(t *testing.T) {
	tr := newActivityTracker()
	tr.observe(&TextEvent{})
	tr.observe(&ToolUseEvent{ID: "t1"})
	// ResultEvent wipes pending counter and returns to idle.
	trans := tr.observe(&ResultEvent{})
	if trans == nil || trans.State != ActivityIdle {
		t.Fatalf("ResultEvent didn't transition to idle: %+v", trans)
	}
	// Next turn: markQuery works again.
	if tr.markQuery() == nil {
		t.Error("markQuery after idle reset should emit")
	}
}

func TestActivityTrackerFatalErrorResets(t *testing.T) {
	tr := newActivityTracker()
	tr.observe(&TextEvent{})
	tr.observe(&ToolUseEvent{ID: "t1"})
	if trans := tr.observe(&ErrorEvent{Fatal: false}); trans != nil {
		t.Errorf("non-fatal error emitted transition: %+v", trans)
	}
	trans := tr.observe(&ErrorEvent{Fatal: true})
	if trans == nil || trans.State != ActivityIdle {
		t.Fatalf("fatal error didn't transition to idle: %+v", trans)
	}
}

// Verifies ParseEvents emits CLIStateChangeEvent before the triggering event
// and that the full transition sequence for a tool-using turn is correct.
func TestParseEventsEmitsStateChanges(t *testing.T) {
	input := `{"type":"system","subtype":"init","session_id":"s","model":"sonnet"}
{"type":"assistant","message":{"content":[{"type":"text","text":"let me read"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}
{"type":"result","subtype":"success","session_id":"s","total_cost_usd":0.01,"usage":{"input_tokens":10,"output_tokens":5}}
`
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		ParseEvents(context.Background(), strings.NewReader(input), ch)
	}()
	var seq []string
	for e := range ch {
		switch v := e.(type) {
		case *CLIStateChangeEvent:
			seq = append(seq, "state:"+string(v.State))
		case *InitEvent:
			seq = append(seq, "init")
		case *TurnEvent:
			seq = append(seq, "turn")
		case *TextEvent:
			seq = append(seq, "text")
		case *ToolUseEvent:
			seq = append(seq, "tool_use")
		case *UserEvent:
			seq = append(seq, "user")
		case *ResultEvent:
			seq = append(seq, "result")
		}
	}
	want := []string{
		"init",
		"state:thinking", "turn", // first TurnEvent triggers thinking
		"text",
		"turn", // each top-level assistant message starts a new TurnEvent
		"state:awaiting_tool_result", "tool_use",
		"state:thinking", "user", // user (tool_result) pairs the tool_use
		"turn", "text",
		"state:idle", "result",
	}
	if len(seq) != len(want) {
		t.Fatalf("sequence mismatch:\ngot  %v\nwant %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Errorf("seq[%d] = %q, want %q", i, seq[i], want[i])
		}
	}
}
