package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// adoptLoginShellPath gives the process (and so the commands malachi runs)
// the PATH of the user's login shell. On macOS an app started from the Dock
// or Finder gets launchd's bare /usr/bin:/bin:/usr/sbin:/sbin, without
// Homebrew, Go or Node. Linux desktop sessions already pass the user's PATH.
func adoptLoginShellPath() {
	if runtime.GOOS != "darwin" {
		return
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh" // macOS's default
	}
	// Interactive too, since many people set PATH in .zshrc; fall back to
	// a plain login shell if that fails or hangs on a prompt.
	for _, flags := range [][]string{{"-i", "-l"}, {"-l"}} {
		if p := shellPath(shell, flags, 3*time.Second); p != "" {
			_ = os.Setenv("PATH", p)
			return
		}
	}
}

const pathMarker = "__MALACHI_PATH__"

// shellPath runs shell with flags and returns the PATH it reports, or "".
// The value is fenced by markers so whatever the shell's startup files
// print is ignored. Quoted "$PATH" is colon-joined in sh, bash, zsh and
// fish alike.
func shellPath(shell string, flags []string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	args := append(append([]string{}, flags...), "-c", `printf '`+pathMarker+`%s`+pathMarker+`' "$PATH"`)
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	parts := bytes.Split(out, []byte(pathMarker))
	if len(parts) < 3 {
		return ""
	}
	return string(bytes.TrimSpace(parts[len(parts)-2]))
}

// appDataDir is where the app keeps data of its own (the chat directory):
// ~/Library/Application Support/malachi-desktop on macOS, else
// $XDG_DATA_HOME/malachi-desktop (default ~/.local/share/malachi-desktop).
func appDataDir() (string, error) {
	if runtime.GOOS == "darwin" {
		base, err := os.UserConfigDir() // ~/Library/Application Support
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "malachi-desktop"), nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "malachi-desktop"), nil
}
