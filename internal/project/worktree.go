package project

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var (
	branchBadRE  = regexp.MustCompile(`[^a-z0-9-]+`)
	branchDashRE = regexp.MustCompile(`-+`)
)

// WorktreeHost derives a safe DNS label from a git branch. Slashes and
// underscores become dashes, collisions are avoided with a short deterministic
// hash, and the result is always under 63 characters (DNS label limit).
func WorktreeHost(branch, base string) string {
	label := strings.ToLower(strings.TrimSpace(branch))
	label = branchBadRE.ReplaceAllString(label, "-")
	label = strings.Trim(label, "-")
	label = branchDashRE.ReplaceAllString(label, "-")
	if label == "" {
		label = "worktree"
	}
	sum := sha256.Sum256([]byte(branch))
	suffix := hex.EncodeToString(sum[:])[:6]
	max := 63 - len(suffix) - 1
	if len(label) > max {
		label = strings.Trim(label[:max], "-")
	}
	host := label + "-" + suffix
	if base != "" {
		return host + "." + strings.TrimPrefix(base, ".")
	}
	return host
}

// WorktreeDBName derives a safe SQL identifier for a worktree's isolated DB.
func WorktreeDBName(branch string) string {
	host := WorktreeHost(branch, "")
	return "wt_" + strings.ReplaceAll(host, "-", "_")
}
