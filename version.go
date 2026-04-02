package claudecli

import (
	"cmp"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// SDKVersion is the version of this Go SDK, sent as CLAUDE_AGENT_SDK_VERSION to the CLI.
const SDKVersion = "0.3.0"

// MinCLIVersion is the minimum Claude CLI version required by this SDK.
const MinCLIVersion = "2.0.0"

var semverRe = regexp.MustCompile(`([0-9]+)\.([0-9]+)\.([0-9]+)`)

// VersionError indicates the CLI version is below minimum.
type VersionError struct {
	Found   string
	Minimum string
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("claudecli: CLI version %s is below minimum %s", e.Found, e.Minimum)
}

// parseSemver extracts major.minor.patch from a version string.
// Returns (major, minor, patch, ok).
func parseSemver(s string) (int, int, int, bool) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return major, minor, patch, true
}

// compareSemver returns -1, 0, or 1 comparing a to b.
func compareSemver(a, b string) int {
	aMaj, aMin, aPat, _ := parseSemver(a)
	bMaj, bMin, bPat, _ := parseSemver(b)
	if c := cmp.Compare(aMaj, bMaj); c != 0 {
		return c
	}
	if c := cmp.Compare(aMin, bMin); c != 0 {
		return c
	}
	return cmp.Compare(aPat, bPat)
}

// CheckCLIVersion runs `claude -v` and returns an error if the version
// is below MinCLIVersion. Returns nil if the version is OK or cannot
// be determined (fail-open).
func CheckCLIVersion(ctx context.Context, binaryPath string) error {
	if binaryPath == "" {
		binaryPath = "claude"
	}
	resolved, err := exec.LookPath(binaryPath)
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, resolved, "-v").Output()
	if err != nil {
		return nil
	}

	maj, min, pat, ok := parseSemver(string(out))
	if !ok {
		return nil
	}

	found := fmt.Sprintf("%d.%d.%d", maj, min, pat)
	if compareSemver(found, MinCLIVersion) < 0 {
		return &VersionError{Found: found, Minimum: MinCLIVersion}
	}
	return nil
}
