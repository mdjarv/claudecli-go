package claudecli

import "time"

// activityTracker derives CLIStateChangeEvent transitions from a stream of
// events. Not goroutine-safe; callers must serialize observe/markQuery calls.
//
// State machine:
//   - idle → thinking: first top-level content event (TextEvent, ThinkingEvent,
//     ToolUseEvent, TurnEvent) of a turn, or explicit markQuery() when the
//     user sends a message.
//   - thinking → awaiting_tool_result: top-level ToolUseEvent emitted.
//   - awaiting_tool_result → awaiting_tool_result: additional top-level
//     ToolUseEvent (pending count increments, no event emitted).
//   - awaiting_tool_result → thinking: last top-level ToolResultEvent
//     (pending count hits zero).
//   - any → idle: ResultEvent or fatal ErrorEvent.
//
// Subagent events (ParentToolUseID != "") do not affect top-level state —
// from the consumer's perspective, the parent Agent tool_use is still
// "awaiting its result" regardless of what happens inside.
type activityTracker struct {
	state           ActivityState
	pendingToolUses int
	now             func() time.Time
}

func newActivityTracker() *activityTracker {
	return &activityTracker{state: ActivityIdle, now: time.Now}
}

// observe returns a CLIStateChangeEvent to emit BEFORE ev, or nil if ev
// does not trigger a state transition.
func (t *activityTracker) observe(ev Event) *CLIStateChangeEvent {
	next := t.state
	switch e := ev.(type) {
	case *TurnEvent:
		if t.state == ActivityIdle {
			next = ActivityThinking
		}
	case *TextEvent:
		if e.ParentToolUseID == "" && t.state == ActivityIdle {
			next = ActivityThinking
		}
	case *ThinkingEvent:
		if e.ParentToolUseID == "" && t.state == ActivityIdle {
			next = ActivityThinking
		}
	case *ToolUseEvent:
		if e.ParentToolUseID == "" {
			t.pendingToolUses++
			next = ActivityAwaitingToolResult
		}
	case *ToolResultEvent:
		if e.ParentToolUseID == "" {
			if t.pendingToolUses > 0 {
				t.pendingToolUses--
			}
			if t.pendingToolUses == 0 {
				next = ActivityThinking
			}
		}
	case *UserEvent:
		// Top-level tool_result blocks arrive inside user events too (CLI
		// feeds tool output back to the model). Count tool_result blocks
		// so pairing stays balanced when tool_use came via an assistant
		// message and the result comes via a user message.
		if e.ParentToolUseID == "" {
			for _, b := range e.Content {
				if b.Type == "tool_result" && t.pendingToolUses > 0 {
					t.pendingToolUses--
				}
			}
			if t.pendingToolUses == 0 && t.state == ActivityAwaitingToolResult {
				next = ActivityThinking
			}
		}
	case *ResultEvent:
		t.pendingToolUses = 0
		next = ActivityIdle
	case *ErrorEvent:
		if e.Fatal {
			t.pendingToolUses = 0
			next = ActivityIdle
		}
	}
	if next == t.state {
		return nil
	}
	t.state = next
	return &CLIStateChangeEvent{State: next, At: t.now()}
}

// markQuery returns a CLIStateChangeEvent to ActivityThinking if the
// tracker is currently idle. Used by Session on Query/SendMessage so the
// transition is visible before the CLI emits anything.
func (t *activityTracker) markQuery() *CLIStateChangeEvent {
	if t.state != ActivityIdle {
		return nil
	}
	t.state = ActivityThinking
	return &CLIStateChangeEvent{State: ActivityThinking, At: t.now()}
}

// State returns the current activity state.
func (t *activityTracker) State() ActivityState {
	return t.state
}
