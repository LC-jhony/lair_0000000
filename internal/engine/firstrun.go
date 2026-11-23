package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/php"
)

// FirstRun performs the unprivileged part of a fresh installation: it ensures
// one PHP line, one Node LTS, the Composer phar and the default database tool
// (SQLite) are present. Each downloadable component is started as a normal
// installer job so the UI can show real progress. The privileged DNS/host
// virtual script is rendered separately by `lair install` and always requires
// user confirmation.
func (e *Engine) FirstRun(ctx context.Context) ([]*installer.Job, error) {
	jobs := []*installer.Job{}

	installedPHP, err := php.Installed(e.Paths)
	if err != nil {
		return nil, err
	}
	if len(installedPHP) == 0 {
		job, err := e.InstallPHP(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("install default PHP: %w", err)
		}
		jobs = append(jobs, job)
	}

	// nvm versions installed outside Lair are visible to ResolveTool but are
	// not managed here; a fresh setup must still install its own Node.
	if !hasCatalogNode(e.Paths.Node) {
		job, err := e.InstallNode(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("install default Node: %w", err)
		}
		jobs = append(jobs, job)
	}

	if job, err := e.EnsureDefaultDatabase(ctx); err != nil {
		return nil, fmt.Errorf("install default database: %w", err)
	} else if job != nil {
		jobs = append(jobs, job)
	}

	// Composer is a single verified phar, not a long-running job. Install it
	// here too so callers that do not run Bootstrap still end up with a usable
	// toolchain.
	if _, err := os.Stat(filepath.Join(e.Paths.Tools, "composer.phar")); err != nil {
		if _, err := e.InstallComposer(ctx); err != nil {
			return nil, fmt.Errorf("install Composer: %w", err)
		}
	}
	return jobs, nil
}

// hasCatalogNode reports whether Lair itself installed a Node version under
// node/<version>, ignoring nvm installations elsewhere on the machine.
func hasCatalogNode(nodeRoot string) bool {
	entries, err := os.ReadDir(nodeRoot)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "global" {
			continue
		}
		dir := filepath.Join(nodeRoot, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "bin", "node")); err == nil {
			return true
		}
		if _, err := os.Stat(filepath.Join(dir, "node.exe")); err == nil {
			return true
		}
	}
	return false
}
