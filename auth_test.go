package claudecli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestAuthStatusResult_JSON(t *testing.T) {
	raw := `{
		"loggedIn": true,
		"authMethod": "claude.ai",
		"apiProvider": "firstParty",
		"email": "user@example.com",
		"orgId": "org-123",
		"orgName": "My Org",
		"subscriptionType": "team"
	}`

	var s AuthStatusResult
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !s.LoggedIn {
		t.Error("expected LoggedIn=true")
	}
	if s.Email != "user@example.com" {
		t.Errorf("Email = %q, want %q", s.Email, "user@example.com")
	}
	if s.OrgName != "My Org" {
		t.Errorf("OrgName = %q, want %q", s.OrgName, "My Org")
	}
	if s.SubscriptionType != "team" {
		t.Errorf("SubscriptionType = %q, want %q", s.SubscriptionType, "team")
	}
}

func TestAuthStatusResult_NotLoggedIn(t *testing.T) {
	raw := `{"loggedIn": false}`

	var s AuthStatusResult
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.LoggedIn {
		t.Error("expected LoggedIn=false")
	}
	if s.Email != "" {
		t.Errorf("Email = %q, want empty", s.Email)
	}
}

func TestExtractLoginURL(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{
			"If the browser didn't open, visit: https://claude.com/cai/oauth/authorize?code=true&client_id=abc",
			"https://claude.com/cai/oauth/authorize?code=true&client_id=abc",
		},
		{
			"If the browser didn't open, visit: https://example.com/auth  ",
			"https://example.com/auth",
		},
		{"Opening browser to sign in…", ""},
		{"random output", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractLoginURL(tt.line)
		if got != tt.want {
			t.Errorf("extractLoginURL(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestAuthLoginOptions_Args(t *testing.T) {
	// Verify that login options produce the expected config.
	var cfg authLoginConfig
	opts := []AuthLoginOption{
		WithAuthMethod(AuthMethodConsole),
		WithSSO(),
		WithLoginEmail("user@example.com"),
	}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.method != AuthMethodConsole {
		t.Errorf("method = %q, want %q", cfg.method, AuthMethodConsole)
	}
	if !cfg.sso {
		t.Error("expected sso=true")
	}
	if cfg.email != "user@example.com" {
		t.Errorf("email = %q, want %q", cfg.email, "user@example.com")
	}
}

func TestAuthLoginOptions_NoBrowser(t *testing.T) {
	var cfg authLoginConfig
	WithNoBrowser()(&cfg)
	if !cfg.noBrowser {
		t.Error("expected noBrowser=true")
	}
}

func TestAuthLoginOptions_Defaults(t *testing.T) {
	var cfg authLoginConfig
	if cfg.method != "" {
		t.Errorf("default method = %q, want empty", cfg.method)
	}
	if cfg.sso {
		t.Error("default sso should be false")
	}
	if cfg.email != "" {
		t.Errorf("default email = %q, want empty", cfg.email)
	}
	if cfg.noBrowser {
		t.Error("default noBrowser should be false")
	}
}

func TestExtractCallbackPort(t *testing.T) {
	tests := []struct {
		name    string
		autoURL string
		want    int
		wantErr bool
	}{
		{
			name:    "valid auto URL",
			autoURL: "https://claude.ai/oauth/authorize?client_id=abc&redirect_uri=http%3A%2F%2Flocalhost%3A12345%2Fcallback&state=xyz",
			want:    12345,
		},
		{
			name:    "high port",
			autoURL: "https://claude.ai/oauth/authorize?redirect_uri=http%3A%2F%2Flocalhost%3A59876%2Fcallback",
			want:    59876,
		},
		{
			name:    "missing redirect_uri",
			autoURL: "https://claude.ai/oauth/authorize?client_id=abc",
			wantErr: true,
		},
		{
			name:    "redirect_uri without port",
			autoURL: "https://claude.ai/oauth/authorize?redirect_uri=http%3A%2F%2Flocalhost%2Fcallback",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			autoURL: "://not-a-url",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractCallbackPort(tt.autoURL)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got port=%d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("port = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWaitForAutoURL(t *testing.T) {
	t.Run("file exists immediately", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "url")
		os.WriteFile(f, []byte("https://example.com/auth"), 0644)

		got, err := waitForAutoURL(f, time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "https://example.com/auth" {
			t.Errorf("got %q, want %q", got, "https://example.com/auth")
		}
	})

	t.Run("file appears after delay", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "url")

		go func() {
			time.Sleep(100 * time.Millisecond)
			os.WriteFile(f, []byte("https://delayed.com"), 0644)
		}()

		got, err := waitForAutoURL(f, 2*time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "https://delayed.com" {
			t.Errorf("got %q, want %q", got, "https://delayed.com")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "nonexistent")

		_, err := waitForAutoURL(f, 100*time.Millisecond)
		if err == nil {
			t.Error("expected timeout error")
		}
	})
}

func TestSubmitCode(t *testing.T) {
	t.Run("code with hash state", func(t *testing.T) {
		var mu sync.Mutex
		var gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			gotPath = r.URL.RequestURI()
			mu.Unlock()
			w.WriteHeader(200)
		}))
		defer srv.Close()

		u, _ := url.Parse(srv.URL)
		port, _ := strconv.Atoi(u.Port())

		lp := &LoginProcess{
			URL:          "https://claude.ai/oauth/authorize?state=url-state&client_id=abc",
			callbackPort: port,
		}

		if err := lp.SubmitCode("mycode#override-state"); err != nil {
			t.Fatalf("SubmitCode: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		want := "/callback?code=mycode&state=override-state"
		if gotPath != want {
			t.Errorf("request path = %q, want %q", gotPath, want)
		}
	})

	t.Run("plain code uses URL state", func(t *testing.T) {
		var mu sync.Mutex
		var gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			gotPath = r.URL.RequestURI()
			mu.Unlock()
			w.WriteHeader(200)
		}))
		defer srv.Close()

		u, _ := url.Parse(srv.URL)
		port, _ := strconv.Atoi(u.Port())

		lp := &LoginProcess{
			URL:          "https://claude.ai/oauth/authorize?state=from-url&client_id=abc",
			callbackPort: port,
		}

		if err := lp.SubmitCode("justthecode"); err != nil {
			t.Fatalf("SubmitCode: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		want := "/callback?code=justthecode&state=from-url"
		if gotPath != want {
			t.Errorf("request path = %q, want %q", gotPath, want)
		}
	})

	t.Run("no port returns error", func(t *testing.T) {
		lp := &LoginProcess{URL: "https://example.com"}
		err := lp.SubmitCode("code")
		if err == nil {
			t.Error("expected error for zero callbackPort")
		}
	})

	t.Run("no state returns error", func(t *testing.T) {
		lp := &LoginProcess{
			URL:          "https://claude.ai/oauth/authorize?client_id=abc",
			callbackPort: 9999,
		}
		err := lp.SubmitCode("codeonly")
		if err == nil {
			t.Error("expected error for missing state")
		}
	})

	t.Run("non-200 returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			w.Write([]byte("bad request"))
		}))
		defer srv.Close()

		u, _ := url.Parse(srv.URL)
		port, _ := strconv.Atoi(u.Port())

		lp := &LoginProcess{
			URL:          "https://claude.ai/oauth/authorize?state=s",
			callbackPort: port,
		}
		err := lp.SubmitCode("code#state")
		if err == nil {
			t.Error("expected error for non-200 response")
		}
	})
}

func TestWriteBrowserCaptureScript(t *testing.T) {
	dir := t.TempDir()
	scriptPath, urlFile, err := writeBrowserCaptureScript(dir)
	if err != nil {
		t.Fatalf("writeBrowserCaptureScript: %v", err)
	}

	// Script file exists and is executable.
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat script: %v", err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Error("script is not executable")
	}

	// URL file path is inside the temp dir.
	if filepath.Dir(urlFile) != dir {
		t.Errorf("urlFile %q not in dir %q", urlFile, dir)
	}
}
