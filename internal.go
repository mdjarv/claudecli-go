package claudecli

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const maxStderrLines = 1000

func scanStderr(ctx context.Context, proc *Process, events chan<- Event, callback func(string)) (*[]string, <-chan struct{}) {
	var lines []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				select {
				case events <- &ErrorEvent{
					Err:   fmt.Errorf("stderr goroutine panic: %v", r),
					Fatal: true,
				}:
				case <-ctx.Done():
				}
			}
		}()
		scanner := bufio.NewScanner(proc.Stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if len(lines) < maxStderrLines {
				lines = append(lines, line)
			} else {
				// Keep most recent lines by shifting.
				copy(lines, lines[1:])
				lines[len(lines)-1] = line
			}
			if callback != nil {
				callback(line)
			}
			select {
			case events <- &StderrEvent{Content: line}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return &lines, done
}

func processExitError(err error, stderr string) *Error {
	details := parseErrorDetails(stderr)
	var msg string
	if details != nil {
		msg = details.Message
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		if msg == "" {
			msg = err.Error()
		}
		return &Error{ExitCode: -1, Stderr: stderr, Message: msg, Details: details}
	}
	return &Error{
		ExitCode: exitErr.ExitCode(),
		Stderr:   stderr,
		Message:  msg,
		Details:  details,
	}
}

// stripCodeFence removes surrounding markdown code fences from text.
// Handles ```json\n...\n```, ```\n...\n```, and leading/trailing whitespace.
// Only matches exactly three backticks optionally followed by a language tag
// (letters/digits only) — four+ backticks or non-alphanumeric suffixes are ignored.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.SplitN(s, "\n", 2)
	if len(lines) < 2 {
		return s
	}
	first := strings.TrimSpace(lines[0])
	if !isOpeningFence(first) {
		return s
	}
	// Find closing fence (may not be last line if model appends commentary)
	rest := s[strings.Index(s, "\n")+1:]
	fenceIdx := -1
	pos := 0
	for {
		nl := strings.Index(rest[pos:], "\n")
		var line string
		if nl < 0 {
			line = rest[pos:]
		} else {
			line = rest[pos : pos+nl]
		}
		if strings.TrimSpace(line) == "```" {
			fenceIdx = pos
			break
		}
		if nl < 0 {
			break
		}
		pos += nl + 1
	}
	if fenceIdx < 0 {
		return s
	}
	// Extract content between fences
	inner := rest[:fenceIdx]
	return strings.TrimSpace(inner)
}

// isOpeningFence returns true for exactly ``` or ```<alphanum lang tag>.
func isOpeningFence(line string) bool {
	if !strings.HasPrefix(line, "```") {
		return false
	}
	tag := line[3:]
	if tag == "" {
		return true
	}
	// Reject 4+ backticks or non-alphanumeric suffixes
	for _, r := range tag {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
