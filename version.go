package main

import (
	_ "embed"
	"regexp"
)

// build/config.yml is the one place the app's version is set: Wails also
// copies it into the macOS plist, Windows metadata and Linux packages.
//
//go:embed build/config.yml
var buildConfig string

// appVersion is info.version from build/config.yml, or "dev".
var appVersion = parseAppVersion(buildConfig)

var configVersion = regexp.MustCompile(`(?m)^info:\s*$(?:\n(?:\s+.*|\s*))*?\n\s+version:\s*["']?([^"'\s#]+)`)

func parseAppVersion(config string) string {
	if m := configVersion.FindStringSubmatch(config); m != nil {
		return m[1]
	}
	return "dev"
}
