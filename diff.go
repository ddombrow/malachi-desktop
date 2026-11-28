package main

import (
	"context"
	"errors"
	"time"
)

// diffTimeout bounds one diff, so a wedged git cannot leave the pane
// loading for good.
const diffTimeout = 30 * time.Second

// WorkingDiff is what changed in the open project's repository since its
// last commit, for the diff pane: malachi's coding.WorkingTreeDiff.
type WorkingDiff struct {
	// Patch is a git-style unified patch: tracked files' changes, staged or
	// not, then untracked files as new ones.
	Patch string `json:"patch"`
	// Truncated says the patch was cut short to stay within malachi's cap.
	Truncated bool `json:"truncated"`
	// NoRepo says the project is not in a git repository, or there is no
	// git: there is nothing to diff, and that is not an error.
	NoRepo bool `json:"noRepo"`
	// Untracked are the paths in Patch that git does not track yet.
	Untracked []string `json:"untracked"`
}

// WorkingTreeDiff diffs the open project's working tree against HEAD.
// The RPC protocol has no command for it, so this asks the session the
// server is driving now (new_session and switch_session replace it)
// directly; malachi runs git as its git tool does, sandboxed and
// read-only, so no rule is bypassed.
func (m *MalachiService) WorkingTreeDiff() (WorkingDiff, error) {
	m.mu.Lock()
	p := m.proc
	m.mu.Unlock()
	if p == nil {
		return WorkingDiff{}, errors.New("no session is open")
	}
	ctx, cancel := context.WithTimeout(context.Background(), diffTimeout)
	defer cancel()
	d, err := p.sv.Session().WorkingTreeDiff(ctx)
	if err != nil {
		return WorkingDiff{}, err
	}
	untracked := d.Untracked
	if untracked == nil {
		untracked = []string{}
	}
	return WorkingDiff{Patch: d.Patch, Truncated: d.Truncated, NoRepo: d.NoRepo, Untracked: untracked}, nil
}
