package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ddombrow/malachi/ai/fake"
	"github.com/ddombrow/malachi/coding"
	"github.com/ddombrow/malachi/rpc"
)

// The diff pane's binding diffs the open session's repository through
// malachi, untracked files included.
func TestWorkingTreeDiffBinding(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("a.txt", "one\ntwo\n")
	git("add", ".")
	git("commit", "-qm", "init")
	write("a.txt", "one\nTWO\n")
	write("new.txt", "fresh\n")

	m := &MalachiService{}
	if _, err := m.WorkingTreeDiff(); err == nil {
		t.Fatal("diff with no session open: want an error")
	}

	s, err := coding.Open(coding.Options{Cwd: dir, Home: t.TempDir(), Settings: &coding.Settings{}, Provider: fake.New(), NoSession: true, Trust: "no"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if st := s.Sandbox(); st.Enabled && st.Unavailable != nil {
		t.Skipf("sandbox not usable here: %+v", st.Unavailable)
	}
	m.proc = &rpcProc{sv: rpc.New(s, strings.NewReader(""), io.Discard)}

	d, err := m.WorkingTreeDiff()
	if err != nil {
		t.Fatal(err)
	}
	if d.NoRepo || d.Truncated {
		t.Fatalf("%+v", d)
	}
	for _, want := range []string{"-two\n+TWO\n", "+++ b/new.txt\n"} {
		if !strings.Contains(d.Patch, want) {
			t.Errorf("missing %q in:\n%s", want, d.Patch)
		}
	}
	if !slices.Equal(d.Untracked, []string{"new.txt"}) {
		t.Errorf("untracked = %q", d.Untracked)
	}
}
