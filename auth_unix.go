//go:build !windows

package claudecli

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeBrowserCaptureScript creates a shell script that captures the URL passed
// by the CLI's browser-opening mechanism instead of opening a real browser.
// Returns the script path (to set as BROWSER env) and the file where the
// captured URL will be written.
func writeBrowserCaptureScript(dir string) (scriptPath, urlFile string, err error) {
	urlFile = filepath.Join(dir, "browser_url")
	scriptPath = filepath.Join(dir, "browser.sh")
	content := fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$1\" > %s\n", urlFile)
	if err := os.WriteFile(scriptPath, []byte(content), 0700); err != nil {
		return "", "", fmt.Errorf("write browser capture script: %w", err)
	}
	return scriptPath, urlFile, nil
}
