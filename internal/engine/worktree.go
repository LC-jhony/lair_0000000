package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/gitworktree"
	"github.com/lair-sh/lair/internal/project"
)

// WorktreeRequest is the wire payload of the `worktree` daemon action.
type WorktreeRequest struct {
	Op   string `json:"op"` // sync|list|provision|prune
	Path string `json:"path,omitempty"`
	Site string `json:"site,omitempty"`
}

// WorktreeInfo describes a provisioned branch site.
type WorktreeInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Site   bool   `json:"site"`
}

// WorktreeResult is returned by worktree operations.
type WorktreeResult struct {
	Worktrees []WorktreeInfo `json:"worktrees,omitempty"`
	Output    string         `json:"output,omitempty"`
}

// SyncWorktrees discovers git worktrees under the configured project folders,
// registers Lair-owned branch sites and prunes ones whose branch no longer
// exists. It is idempotent and never touches manually linked sites.
func (e *Engine) SyncWorktrees(ctx context.Context) (WorktreeResult, error) {
	c, err := e.Config.View()
	if err != nil {
		return WorktreeResult{}, err
	}
	seen := map[string]bool{}
	for _, dir := range c.ProjectDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			linked, err := gitworktree.IsWorktree(ctx, path)
			if err != nil || !linked {
				continue
			}
			common, err := gitworktree.CommonDir(ctx, path)
			if err != nil {
				continue
			}
			wts, err := gitworktree.List(ctx, path)
			if err != nil {
				continue
			}
			for _, w := range wts {
				if w.Branch == "" || w.Bare || w.Path == path {
					continue
				}
				host := project.WorktreeHost(w.Branch, c.Web.Domain)
				seen[host] = true
				view, err := e.Config.View()
				if err != nil {
					continue
				}
				site, ok := view.Sites[host]
				if ok && !site.Worktree {
					continue // never take over a manually linked site
				}
				if !ok {
					if err := e.Link(host, w.Path); err != nil {
						continue
					}
					if err := e.Config.Update(func(cfg *config.Config) error {
						s := cfg.Sites[host]
						s.Worktree = true
						s.Branch = w.Branch
						s.GitCommonDir = common
						cfg.Sites[host] = s
						return nil
					}); err != nil {
						continue
					}
				}
				// Existing wildcard SANs cover branch subdomains; issue the
				// per-site cert once the vhost exists.
				_ = e.Secure(host)
			}
		}
	}
	// Prune only Lair-created worktree sites that are no longer discovered.
	view, err := e.Config.View()
	if err != nil {
		return WorktreeResult{}, err
	}
	for name, site := range view.Sites {
		if !site.Worktree || seen[name] {
			continue
		}
		// Auto cleanup is opt-in; explicit WorktreeControl prunes regardless.
		if !view.Worktrees.Auto {
			continue
		}
		if err := e.Unlink(name); err != nil {
			continue
		}
		_ = os.Remove(filepath.Join(e.Paths.Certs, name+".pem"))
		_ = os.Remove(filepath.Join(e.Paths.Certs, name+"-key.pem"))
	}
	e.reloadSites()
	return e.ListWorktrees()
}

// ListWorktrees returns the sites Lair created from git branches.
func (e *Engine) ListWorktrees() (WorktreeResult, error) {
	c, err := e.Config.View()
	if err != nil {
		return WorktreeResult{}, err
	}
	out := WorktreeResult{}
	for name, site := range c.Sites {
		if !site.Worktree {
			continue
		}
		out.Worktrees = append(out.Worktrees, WorktreeInfo{
			Name: name, Path: site.Path, Branch: site.Branch, Site: true,
		})
	}
	return out, nil
}

// ProvisionWorktree registers one explicit worktree path with its branch
// domain, pin and isolated database name. No git state is modified.
func (e *Engine) ProvisionWorktree(ctx context.Context, path string) (WorktreeResult, error) {
	if strings.TrimSpace(path) == "" {
		return WorktreeResult{}, fmt.Errorf("falta la ruta del worktree")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return WorktreeResult{}, err
	}
	wts, err := gitworktree.List(ctx, abs)
	if err != nil {
		return WorktreeResult{}, err
	}
	c, err := e.Config.View()
	if err != nil {
		return WorktreeResult{}, err
	}
	for _, w := range wts {
		if w.Path != abs || w.Branch == "" {
			continue
		}
		host := project.WorktreeHost(w.Branch, c.Web.Domain)
		if err := e.Link(host, w.Path); err != nil {
			return WorktreeResult{}, err
		}
		if err := e.Config.Update(func(cfg *config.Config) error {
			s := cfg.Sites[host]
			s.Worktree = true
			s.Branch = w.Branch
			s.GitCommonDir, _ = gitworktree.CommonDir(ctx, abs)
			s.DatabaseName = project.WorktreeDBName(w.Branch)
			cfg.Sites[host] = s
			return nil
		}); err != nil {
			return WorktreeResult{}, err
		}
		_ = e.Secure(host)
		return WorktreeResult{Output: "worktree registrado como " + host}, nil
	}
	return WorktreeResult{}, fmt.Errorf("%s no es un worktree de git", abs)
}

// WorktreeControl is the daemon-facing entry point.
func (e *Engine) WorktreeControl(ctx context.Context, req WorktreeRequest) (WorktreeResult, error) {
	switch req.Op {
	case "", "list":
		return e.ListWorktrees()
	case "sync":
		return e.SyncWorktrees(ctx)
	case "provision":
		return e.ProvisionWorktree(ctx, req.Path)
	case "prune":
		return e.SyncWorktrees(ctx)
	default:
		return WorktreeResult{}, fmt.Errorf("operación worktree desconocida %q", req.Op)
	}
}
