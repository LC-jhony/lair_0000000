package version

import (
	"strings"
	"testing"
)

func TestStringIncludesBuildIdentity(t *testing.T) {
	oldV, oldC, oldD := BuildVersion, BuildCommit, BuildDate
	t.Cleanup(func() { BuildVersion, BuildCommit, BuildDate = oldV, oldC, oldD })

	BuildVersion, BuildCommit, BuildDate = "1.0.3", "9870c6f", "2026-11-24"
	got := String()
	for _, want := range []string{"lair 1.0.3", "9870c6f", "2026-11-24"} {
		if !strings.Contains(got, want) {
			t.Fatalf("String() = %q, missing %q", got, want)
		}
	}

	BuildVersion, BuildCommit, BuildDate = "", "unknown", "unknown"
	if got := String(); got != "lair dev" {
		t.Fatalf("String() with no build data = %q, want %q", got, "lair dev")
	}
}
