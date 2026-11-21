package main

import "testing"

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
