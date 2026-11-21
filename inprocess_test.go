package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddombrow/malachi/agent"
	"github.com/ddombrow/malachi/ai/fake"
	"github.com/ddombrow/malachi/coding"
	"github.com/ddombrow/malachi/sandbox"
)

// TestMain mirrors main: on Linux a sandboxed command re-runs this binary
// in helper mode, which must be handled before tests run.
func TestMain(m *testing.M) {
	sandbox.MaybeRunHelper()
	os.Exit(m.Run())
}

// An in-process session runs commands through the sandbox, with this
// binary as its helper: writes in the project work, while the user's home
// (private: readable only where allowed) cannot be listed.
func TestInProcessSandboxedCommand(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	cmd := "echo hi > inside.txt && (ls " + userHome + " >/dev/null 2>&1 && echo LEAKED || echo BLOCKED)"
	p := fake.New(
		fake.ToolCalls("Running.", fake.Call{ID: "c1", Name: "bash", Args: map[string]any{"command": cmd}}),
		fake.Text("done"),
	)
	s, err := coding.Open(coding.Options{Cwd: cwd, Home: home, Settings: &coding.Settings{}, Provider: p, NoSession: true, Trust: "no"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if st := s.Sandbox(); !st.Enabled || st.Unavailable != nil {
		t.Skipf("sandbox not usable here: %+v", st.Unavailable)
	}
	if err := s.Prompt(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	var out string
	for _, m := range s.Harness.Messages() {
		if r, ok := m.(*agent.ToolResultMessage); ok {
			out = r.Text()
		}
	}
	if !strings.Contains(out, "BLOCKED") {
		t.Fatalf("command output = %q, want BLOCKED", out)
	}
	if b, err := os.ReadFile(filepath.Join(cwd, "inside.txt")); err != nil || strings.TrimSpace(string(b)) != "hi" {
		t.Fatalf("inside.txt = %q, %v", b, err)
	}
}

// Sessions open straight in the saved code mode (malachi v0.9.2+).
func TestOpenInCodeMode(t *testing.T) {
	s, err := coding.Open(coding.Options{
		Cwd: t.TempDir(), Home: t.TempDir(), Settings: &coding.Settings{},
		Provider: fake.New(fake.Text("hi")), NoSession: true, Trust: "no", CodeMode: "explore",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := s.CodeMode().Mode; got != "explore" {
		t.Fatalf("code mode = %q, want explore", got)
	}
}
