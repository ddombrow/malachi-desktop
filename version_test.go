package main

import "testing"

func TestParseAppVersion(t *testing.T) {
	cfg := "version: '3'\n\n# comment\ninfo:\n  companyName: \"x\"\n  version: \"1.2.3\" # The application version\n\ndev_mode:\n  root_path: .\n"
	if got := parseAppVersion(cfg); got != "1.2.3" {
		t.Fatalf("parseAppVersion = %q", got)
	}
	if got := parseAppVersion("version: '3'\n"); got != "dev" {
		t.Fatalf("no info block: %q", got)
	}
	if appVersion == "dev" {
		t.Fatal("build/config.yml's info.version was not found")
	}
}
