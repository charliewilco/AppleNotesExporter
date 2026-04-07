package notes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	scripts "github.com/charliewilco/AppleNotesExporter/scripts"
)

var ErrUnavailable = errors.New("apple-notes-md requires macOS with Notes.app available")

func Fetch(ctx context.Context) ([]Note, error) {
	payload, err := FetchRaw(ctx)
	if err != nil {
		return nil, err
	}

	return ParsePayload(payload)
}

func FetchRaw(ctx context.Context) ([]byte, error) {
	if runtime.GOOS != "darwin" {
		return nil, ErrUnavailable
	}

	if _, err := exec.LookPath("osascript"); err != nil {
		return nil, ErrUnavailable
	}

	scriptPath, cleanup, err := writeEmbeddedScript()
	if err != nil {
		return nil, fmt.Errorf("prepare AppleScript bridge: %w", err)
	}
	defer cleanup()

	command := exec.CommandContext(ctx, "osascript", scriptPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}

		return nil, fmt.Errorf("%w: %s", ErrUnavailable, message)
	}

	return stdout.Bytes(), nil
}

func writeEmbeddedScript() (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "apple-notes-md-*")
	if err != nil {
		return "", nil, err
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	scriptPath := filepath.Join(tempDir, "fetch_notes.applescript")
	scriptSource := strings.TrimSpace(scripts.FetchNotesAppleScript) + "\n"
	if err := os.WriteFile(scriptPath, []byte(scriptSource), 0o600); err != nil {
		cleanup()
		return "", nil, err
	}

	return scriptPath, cleanup, nil
}
