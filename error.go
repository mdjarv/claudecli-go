package claudecli

import "fmt"

// Error represents a CLI process failure with context.
type Error struct {
	ExitCode int
	Stderr   string
	Message  string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("claudecli: exit %d: %s", e.ExitCode, e.Message)
	}
	if e.Stderr != "" {
		return fmt.Sprintf("claudecli: exit %d: %s", e.ExitCode, e.Stderr)
	}
	return fmt.Sprintf("claudecli: exit %d", e.ExitCode)
}
