package claudecli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Client wraps a Claude CLI executor with default options.
type Client struct {
	executor Executor
	defaults []Option
}

// New creates a client. Pass options to set defaults for all calls.
// Use WithBinaryPath to override the CLI binary location.
func New(defaults ...Option) *Client {
	resolved := resolveOptions(defaults, nil)
	executor := NewLocalExecutor()
	if resolved.binaryPath != "" {
		executor.BinaryPath = resolved.binaryPath
	}
	return &Client{
		executor: executor,
		defaults: defaults,
	}
}

// NewWithExecutor creates a client with a specific executor and default options.
func NewWithExecutor(executor Executor, defaults ...Option) *Client {
	return &Client{
		executor: executor,
		defaults: defaults,
	}
}

// Run starts a streaming Claude session. Returns a Stream for event consumption.
func (c *Client) Run(ctx context.Context, prompt string, opts ...Option) *Stream {
	ctx, cancel := context.WithCancel(ctx)
	resolved := resolveOptions(c.defaults, opts)
	args := resolved.buildArgs()

	events := make(chan Event, 64)
	done := make(chan struct{})
	stream := newStream(ctx, events, done, cancel)

	// Emit StartEvent before process launch
	events <- &StartEvent{
		Model:   resolved.model,
		Args:    args,
		WorkDir: resolved.workDir,
	}

	go func() {
		defer close(done)
		defer close(events)

		cfg := &StartConfig{
			Args:    args,
			Stdin:   strings.NewReader(prompt),
			Env:     resolved.env,
			WorkDir: resolved.workDir,
		}

		proc, err := c.executor.Start(ctx, cfg)
		if err != nil {
			events <- &ErrorEvent{Err: fmt.Errorf("start: %w", err), Fatal: true}
			return
		}

		// Parse stderr in background, collect lines for error reporting
		var stderrLines []string
		stderrDone := make(chan struct{})
		go func() {
			defer close(stderrDone)
			defer func() {
				if r := recover(); r != nil {
					events <- &ErrorEvent{
						Err:   fmt.Errorf("stderr goroutine panic: %v", r),
						Fatal: true,
					}
				}
			}()
			scanner := bufio.NewScanner(proc.Stderr)
			for scanner.Scan() {
				line := scanner.Text()
				stderrLines = append(stderrLines, line)
				events <- &StderrEvent{Content: line}
			}
		}()

		ParseEvents(proc.Stdout, events)

		// Wait for stderr goroutine
		<-stderrDone

		if err := proc.Wait(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				events <- &ErrorEvent{
					Err: &Error{
						ExitCode: exitErr.ExitCode(),
						Stderr:   strings.Join(stderrLines, "\n"),
					},
					Fatal: true,
				}
			} else {
				events <- &ErrorEvent{Err: err, Fatal: true}
			}
		}
	}()

	return stream
}

// RunText runs a prompt and returns the accumulated text output.
func (c *Client) RunText(ctx context.Context, prompt string, opts ...Option) (string, *ResultEvent, error) {
	stream := c.Run(ctx, prompt, opts...)
	result, err := stream.Wait()
	if err != nil {
		return "", result, err
	}
	if result == nil {
		return "", nil, ErrEmptyOutput
	}
	return result.Text, result, nil
}

// RunJSON runs a prompt and unmarshals the text output into T.
func RunJSON[T any](ctx context.Context, c *Client, prompt string, opts ...Option) (T, *ResultEvent, error) {
	var zero T
	text, result, err := c.RunText(ctx, prompt, opts...)
	if err != nil {
		return zero, result, err
	}
	if err := json.Unmarshal([]byte(text), &zero); err != nil {
		return zero, result, fmt.Errorf("unmarshal response: %w", err)
	}
	return zero, result, nil
}

// Package-level shortcuts for one-off use without constructing a client.

var defaultClient = New()

// Run starts a streaming session using the default local executor.
func Run(ctx context.Context, prompt string, opts ...Option) *Stream {
	return defaultClient.Run(ctx, prompt, opts...)
}

// RunText runs a prompt and returns text using the default local executor.
func RunText(ctx context.Context, prompt string, opts ...Option) (string, *ResultEvent, error) {
	return defaultClient.RunText(ctx, prompt, opts...)
}
