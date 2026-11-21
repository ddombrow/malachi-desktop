package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectLike(t *testing.T) {
	for dir, want := range map[string]bool{
		"/home/u/code/malachi":         true,
		"/tmp/x":                       false,
		"/tmp":                         false,
		"/tmpfoo/x":                    true,
		"/home/u/.local/share/a/agent": false,
	} {
		if got := projectLike(dir); got != want {
			t.Errorf("projectLike(%q) = %v, want %v", dir, got, want)
		}
	}
}

func TestResumableSkipsUnwrittenSession(t *testing.T) {
	dir := t.TempDir()
	if got := resumable(filepath.Join(dir, "not-yet.jsonl")); got != "" {
		t.Fatalf("unwritten session: resumable = %q, want empty", got)
	}
	saved := filepath.Join(dir, "saved.jsonl")
	if err := os.WriteFile(saved, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resumable(saved); got != saved {
		t.Fatalf("saved session: resumable = %q", got)
	}
	if got := resumable(dir); got != "" {
		t.Fatalf("directory: resumable = %q", got)
	}
}
