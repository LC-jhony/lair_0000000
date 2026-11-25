package project

import (
	"strings"
	"testing"
)

func TestWorktreeHostIsSafeAndStable(t *testing.T) {
	a := WorktreeHost("feature/MI-cosa", "test")
	b := WorktreeHost("feature/MI-cosa", "test")
	if a != b || !strings.HasSuffix(a, ".test") {
		t.Fatalf("host = %q vs %q", a, b)
	}
	if strings.ContainsAny(a, "/_ ") {
		t.Fatalf("unsafe host: %q", a)
	}
	if WorktreeHost("feature/foo", "test") == WorktreeHost("feature-foo", "test") {
		t.Fatal("colliding branches produced the same host")
	}
	long := WorktreeHost(strings.Repeat("a", 100), "test")
	if len(strings.Split(long, ".")[0]) > 63 {
		t.Fatalf("label too long: %q", long)
	}
}

func TestWorktreeDBName(t *testing.T) {
	if name := WorktreeDBName("feature/foo"); !strings.HasPrefix(name, "wt_") || strings.Contains(name, "-") {
		t.Fatalf("db name = %q", name)
	}
}
