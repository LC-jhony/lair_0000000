package gitworktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestListAndIsWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-qm", "init")

	linked := filepath.Join(t.TempDir(), "feature")
	run("worktree", "add", "-q", "-b", "feature/demo", linked)

	ctx := context.Background()
	main, err := List(ctx, root)
	if err != nil || len(main) != 2 {
		t.Fatalf("worktrees = %d, %v", len(main), err)
	}
	found := false
	for _, w := range main {
		if w.Branch == "feature/demo" && w.Path == linked {
			found = true
		}
	}
	if !found {
		t.Fatalf("linked worktree missing: %+v", main)
	}
	yes, err := IsWorktree(ctx, linked)
	if err != nil || !yes {
		t.Fatalf("IsWorktree(linked) = %v, %v", yes, err)
	}
	yes, err = IsWorktree(ctx, root)
	if err != nil || yes {
		t.Fatalf("IsWorktree(main) = %v, %v", yes, err)
	}
	common, err := CommonDir(ctx, linked)
	if err != nil || !filepath.IsAbs(common) {
		t.Fatalf("CommonDir = %q, %v", common, err)
	}
}
