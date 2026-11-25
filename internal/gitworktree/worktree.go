// Package gitworktree reads git worktree metadata with fixed argv (no shell).
// It never modifies a repository; callers decide what to provision.
package gitworktree

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Info describes one worktree reported by `git worktree list --porcelain`.
type Info struct {
	Path   string
	HEAD   string
	Branch string
	Bare   bool
	Locked bool
}

// List returns the worktrees of the repository at dir. The first entry is the
// main worktree.
func List(ctx context.Context, dir string) ([]Info, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	var result []Info
	var current Info
	flush := func() {
		if current.Path != "" {
			result = append(result, current)
		}
		current = Info{}
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "bare":
			current.Bare = true
		case strings.HasPrefix(line, "locked"):
			current.Locked = true
		}
	}
	flush()
	return result, scanner.Err()
}

// CommonDir returns the repository's shared git dir; it is identical across
// worktrees and can be used to identify them as one repository.
func CommonDir(ctx context.Context, dir string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsWorktree reports whether dir is a linked worktree rather than the main
// checkout: its private git dir differs from the repository's common dir.
func IsWorktree(ctx context.Context, dir string) (bool, error) {
	gitDir, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-dir").Output()
	if err != nil {
		return false, err
	}
	common, err := CommonDir(ctx, dir)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(gitDir)) != common, nil
}
