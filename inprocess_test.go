package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddombrow/malachi/agent"
	"github.com/ddombrow/malachi/ai/fake"
	"github.com/ddombrow/malachi/coding"
	"github.com/ddombrow/malachi/sandbox"
)

// TestMain mirrors main: on Linux a sandboxed command re-runs this binary
// in helper mode, which must be handled before tests run.
func TestMain(m *testing.M) {
	sandbox.MaybeRunHelper()
	runMemoryServer() // malachi starts this binary as the memory MCP server
	os.Exit(m.Run())
}

// isolateAppData gives the test empty settings and app data directories of
// its own. Setting XDG_* alone is not enough: on macOS the app ignores it and
// uses ~/Library/Application Support, so HOME moves too. Only tests that need
// no real HOME may use it; malachi's sandbox protects whatever HOME names.
func isolateAppData(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
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

// With memory on, a session gets malachi-memory served by this binary,
// through malachi's MCP manager and sandbox, and what the agent remembers
// lands in the app's database.
func TestMemoryServerInSession(t *testing.T) {
	isolateAppData(t)
	cfg, err := memoryServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Outside /tmp the sandboxed server could not create its own writable
	// root; the config must have made it.
	if st, err := os.Stat(cfg.Writable[0]); err != nil || !st.IsDir() {
		t.Fatalf("writable root %s not created: %v", cfg.Writable[0], err)
	}
	p := fake.New(
		fake.ToolCalls("Saving.", fake.Call{ID: "m1", Name: "mcp__memory__remember",
			Args: map[string]any{"text": "prefers tabs over spaces", "topics": []any{"style"}}}),
		fake.Text("noted"),
	)
	s, err := coding.Open(coding.Options{
		Cwd: t.TempDir(), Home: t.TempDir(), Settings: &coding.Settings{}, Provider: p,
		NoSession: true, Trust: "no",
		ExtraMCPServers: map[string]coding.MCPServerConfig{memoryServerName: cfg},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	deadline := time.Now().Add(20 * time.Second)
	for {
		var st coding.MCPServerState
		for _, srv := range s.MCPState().Servers {
			if srv.Name == memoryServerName {
				st = srv
			}
		}
		if st.Status == "ready" {
			break
		}
		if st.Status == "failed" || time.Now().After(deadline) {
			t.Fatalf("memory server: status %q, error %q", st.Status, st.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := s.Prompt(context.Background(), "remember that"); err != nil {
		t.Fatal(err)
	}
	for _, m := range s.Harness.Messages() {
		if r, ok := m.(*agent.ToolResultMessage); ok && r.IsError {
			t.Fatalf("tool error: %s", r.Text())
		}
	}
	items, err := (&MalachiService{}).MemoryList("tabs", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Text != "prefers tabs over spaces" || len(items[0].Topics) != 1 {
		t.Fatalf("memories = %+v", items)
	}
}
