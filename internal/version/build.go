// Package version already resolves PHP, Node and service versions. This file
// carries the identity of the lair binary itself. The build injects these
// values with -ldflags -X so a released artifact can be traced back to the
// exact commit it was built from.
package version

import "strings"

var (
	// BuildVersion is the release tag ("1.0.3") or "dev" for local builds.
	// Distinct from the Version type used to resolve PHP/Node versions.
	BuildVersion = "dev"
	// BuildCommit is the short git SHA at build time.
	BuildCommit = "unknown"
	// BuildDate is the commit date (YYYY-MM-DD) at build time.
	BuildDate = "unknown"
)

// String renders the build identity in one line, e.g.
// "lair 1.0.3 (9870c6f, 2026-11-24)".
func String() string {
	v := strings.TrimSpace(BuildVersion)
	if v == "" {
		v = "dev"
	}
	parts := []string{"lair " + v}
	if c := strings.TrimSpace(BuildCommit); c != "" && c != "unknown" {
		parts = append(parts, c)
	}
	if d := strings.TrimSpace(BuildDate); d != "" && d != "unknown" {
		parts = append(parts, d)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + " (" + strings.Join(parts[1:], ", ") + ")"
}
