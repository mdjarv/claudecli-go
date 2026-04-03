package claudecli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// AuthStatusResult represents the authentication state returned by the CLI.
type AuthStatusResult struct {
	LoggedIn         bool   `json:"loggedIn"`
	AuthMethod       string `json:"authMethod,omitempty"`       // e.g. "claude.ai", "api-key"
	APIProvider      string `json:"apiProvider,omitempty"`       // e.g. "firstParty", "bedrock", "vertex"
	Email            string `json:"email,omitempty"`
	OrgID            string `json:"orgId,omitempty"`
	OrgName          string `json:"orgName,omitempty"`
	SubscriptionType string `json:"subscriptionType,omitempty"` // e.g. "team", "pro"
}

// AuthMethod selects the authentication provider for login.
type AuthMethod string

const (
	AuthMethodClaudeAI AuthMethod = "claudeai" // Claude subscription (default)
	AuthMethodConsole  AuthMethod = "console"   // Anthropic Console (API billing)
)

// authLoginConfig holds resolved login options.
type authLoginConfig struct {
	method    AuthMethod
	sso       bool
	email     string
	noBrowser bool
}

// AuthLoginOption configures AuthLogin behavior.
type AuthLoginOption func(*authLoginConfig)

// WithAuthMethod sets the authentication provider.
func WithAuthMethod(m AuthMethod) AuthLoginOption {
	return func(c *authLoginConfig) { c.method = m }
}

// WithSSO forces the SSO login flow.
func WithSSO() AuthLoginOption {
	return func(c *authLoginConfig) { c.sso = true }
}

// WithLoginEmail pre-populates the email on the login page.
func WithLoginEmail(email string) AuthLoginOption {
	return func(c *authLoginConfig) { c.email = email }
}

// WithNoBrowser suppresses the CLI's automatic browser opening by setting
// BROWSER=true in the subprocess environment. Callers that already have the
// login URL (e.g. from LoginProcess.URL) can use this to avoid duplicate tabs.
func WithNoBrowser() AuthLoginOption {
	return func(c *authLoginConfig) { c.noBrowser = true }
}

// LoginProcess represents an in-progress OAuth login.
// The URL field contains the authorization URL for the user to visit.
// Call Wait to block until the login completes or the context is cancelled.
type LoginProcess struct {
	URL          string
	callbackPort int           // port of the CLI's local OAuth callback server
	tmpDir       string        // temp dir for browser capture script; cleaned up on Wait/Cancel
	stdin        io.WriteCloser
	cmd          *exec.Cmd
	done         chan error
}

// Wait blocks until the login process completes. Returns nil on success.
func (p *LoginProcess) Wait() error {
	err := <-p.done
	if p.tmpDir != "" {
		os.RemoveAll(p.tmpDir)
	}
	return err
}

// SubmitCode completes the OAuth flow by submitting an authorization code to
// the CLI's local callback server. The code can be in CODE#STATE format (as
// shown on the platform page) or just the code (state is extracted from the
// login URL). Requires WithNoBrowser to have been used when starting the login.
func (p *LoginProcess) SubmitCode(code string) error {
	if p.callbackPort == 0 {
		return fmt.Errorf("callback port not available (requires WithNoBrowser)")
	}

	// Parse state from the manual URL as default.
	u, err := url.Parse(p.URL)
	if err != nil {
		return fmt.Errorf("parse login URL: %w", err)
	}
	state := u.Query().Get("state")

	// Support CODE#STATE format from the platform page.
	authCode := code
	if parts := strings.SplitN(code, "#", 2); len(parts) == 2 {
		authCode = parts[0]
		state = parts[1]
	}

	if state == "" {
		return fmt.Errorf("no state parameter: provide code as CODE#STATE")
	}

	callbackURL := fmt.Sprintf("http://localhost:%d/callback?code=%s&state=%s",
		p.callbackPort, url.QueryEscape(authCode), url.QueryEscape(state))
	resp, err := http.Get(callbackURL)
	if err != nil {
		return fmt.Errorf("callback request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("callback returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// Cancel terminates the login process.
func (p *LoginProcess) Cancel() error {
	if p.tmpDir != "" {
		os.RemoveAll(p.tmpDir)
	}
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// AuthStatus returns the current authentication state.
func (c *Client) AuthStatus(ctx context.Context) (*AuthStatusResult, error) {
	binary, err := exec.LookPath(c.binaryPath())
	if err != nil {
		return nil, fmt.Errorf("claude binary not found: %w", err)
	}

	out, err := exec.CommandContext(ctx, binary, "auth", "status", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("auth status: %w", err)
	}

	var status AuthStatusResult
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("auth status: parse: %w", err)
	}
	return &status, nil
}

// AuthLogin starts an OAuth login flow. Returns a LoginProcess with the
// authorization URL once it's available. Call Wait on the returned process
// to block until login completes.
func (c *Client) AuthLogin(ctx context.Context, opts ...AuthLoginOption) (*LoginProcess, error) {
	var cfg authLoginConfig
	for _, o := range opts {
		o(&cfg)
	}

	args := []string{"auth", "login"}
	switch cfg.method {
	case AuthMethodConsole:
		args = append(args, "--console")
	case AuthMethodClaudeAI:
		args = append(args, "--claudeai")
	}
	if cfg.sso {
		args = append(args, "--sso")
	}
	if cfg.email != "" {
		args = append(args, "--email", cfg.email)
	}

	binary, err := exec.LookPath(c.binaryPath())
	if err != nil {
		return nil, fmt.Errorf("claude binary not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, binary, args...)

	var tmpDir, urlFile string
	if cfg.noBrowser {
		tmpDir, err = os.MkdirTemp("", "claudecli-auth-*")
		if err != nil {
			return nil, fmt.Errorf("auth login: create temp dir: %w", err)
		}
		scriptPath, uf, err := writeBrowserCaptureScript(tmpDir)
		if err != nil {
			os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("auth login: %w", err)
		}
		urlFile = uf
		cmd.Env = append(os.Environ(), "BROWSER="+scriptPath)
	}

	// Merge stdout and stderr — the URL may appear on either.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("auth login: stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout pipe

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("auth login: stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("auth login: start: %w", err)
	}

	// Scan output for the authorization URL.
	urlCh := make(chan string, 1)
	doneCh := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if u := extractLoginURL(line); u != "" {
				urlCh <- u
			}
		}
	}()

	go func() {
		doneCh <- cmd.Wait()
	}()

	// Wait for either the URL, process exit, or context cancellation.
	select {
	case loginURL := <-urlCh:
		var port int
		if urlFile != "" {
			autoURL, err := waitForAutoURL(urlFile, 10*time.Second)
			if err == nil {
				port, _ = extractCallbackPort(autoURL)
			}
			// Non-fatal: port=0 means SubmitCode will return an error.
		}
		lp := &LoginProcess{
			URL:          loginURL,
			callbackPort: port,
			tmpDir:       tmpDir,
			stdin:        stdinPipe,
			cmd:          cmd,
			done:         doneCh,
		}
		return lp, nil
	case err := <-doneCh:
		os.RemoveAll(tmpDir)
		// Process exited before we got a URL.
		if err != nil {
			return nil, fmt.Errorf("auth login: %w", err)
		}
		// Exited successfully without URL — may already be logged in.
		return nil, nil
	case <-ctx.Done():
		os.RemoveAll(tmpDir)
		_ = cmd.Process.Kill()
		return nil, ctx.Err()
	}
}

// AuthLogout signs out of the current Anthropic account.
func (c *Client) AuthLogout(ctx context.Context) error {
	binary, err := exec.LookPath(c.binaryPath())
	if err != nil {
		return fmt.Errorf("claude binary not found: %w", err)
	}

	out, err := exec.CommandContext(ctx, binary, "auth", "logout").CombinedOutput()
	if err != nil {
		return fmt.Errorf("auth logout: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Package-level auth shortcuts using the default client.

func AuthStatus(ctx context.Context) (*AuthStatusResult, error) {
	return defaultClient.AuthStatus(ctx)
}

func AuthLogin(ctx context.Context, opts ...AuthLoginOption) (*LoginProcess, error) {
	return defaultClient.AuthLogin(ctx, opts...)
}

func AuthLogout(ctx context.Context) error {
	return defaultClient.AuthLogout(ctx)
}

// extractLoginURL extracts the authorization URL from a CLI output line.
func extractLoginURL(line string) string {
	const prefix = "If the browser didn't open, visit: "
	if _, after, ok := strings.Cut(line, prefix); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// extractCallbackPort parses the CLI's auto-open URL and returns the port from
// its redirect_uri parameter (e.g. redirect_uri=http://localhost:PORT/callback).
func extractCallbackPort(autoURL string) (int, error) {
	u, err := url.Parse(autoURL)
	if err != nil {
		return 0, fmt.Errorf("parse auto URL: %w", err)
	}
	redirectURI := u.Query().Get("redirect_uri")
	if redirectURI == "" {
		return 0, fmt.Errorf("no redirect_uri in auto URL")
	}
	ru, err := url.Parse(redirectURI)
	if err != nil {
		return 0, fmt.Errorf("parse redirect_uri: %w", err)
	}
	portStr := ru.Port()
	if portStr == "" {
		return 0, fmt.Errorf("no port in redirect_uri %q", redirectURI)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return port, nil
}

// waitForAutoURL polls for the browser capture file to appear and returns its
// contents. The CLI writes the auto-open URL to this file via the BROWSER script.
func waitForAutoURL(urlFile string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(urlFile)
		if err == nil && len(data) > 0 {
			return string(data), nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", fmt.Errorf("timed out waiting for browser URL capture")
}

