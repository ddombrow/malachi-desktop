package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A shell whose startup files print noise still yields just its PATH.
func TestShellPathIgnoresStartupNoise(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	shell := filepath.Join(t.TempDir(), "chattysh")
	script := "#!/bin/sh\necho 'welcome back!'\nexec /bin/sh \"$@\"\n"
	if err := os.WriteFile(shell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin:/bin:/opt/test/bin")
	if got := shellPath(shell, nil, 5*time.Second); got != "/usr/bin:/bin:/opt/test/bin" {
		t.Fatalf("shellPath = %q", got)
	}
}

func TestShellPathFailureIsEmpty(t *testing.T) {
	if got := shellPath("/nonexistent/shell", []string{"-l"}, time.Second); got != "" {
		t.Fatalf("shellPath = %q, want empty", got)
	}
}
