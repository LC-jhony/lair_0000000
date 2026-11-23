package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/version"
)

// SQLiteToolID is a catalog package that ships the sqlite3 CLI. It is not a
// supervised service: SQLite is embedded in each Laravel project, so there is
// no process, port or data directory to manage here.
const SQLiteToolID = "sqlite3"

func toolExecutable() string {
	if strings.HasPrefix(app.Platform(), "windows-") {
		return "bin/sqlite3.exe"
	}
	return "bin/sqlite3"
}

func (e *Engine) installedToolVersions(id string) []string {
	entries, err := os.ReadDir(e.Paths.Tools)
	if err != nil {
		return []string{}
	}
	out := []string{}
	prefix := id + "-"
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := strings.TrimPrefix(name, prefix)
		idx := strings.LastIndex(rest, "-build")
		if idx <= 0 {
			continue
		}
		ver := rest[:idx]
		if _, err := version.Parse(ver); err == nil {
			out = append(out, ver)
		}
	}
	version.SortNewest(out)
	return out
}

func (e *Engine) installedToolVersion(id string) string {
	data, err := os.ReadFile(filepath.Join(e.Paths.Tools, id+".version"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (e *Engine) toolDir(id, ver string) (string, error) {
	c, err := e.Config.View()
	if err != nil {
		return "", err
	}
	build := c.Installed[id+"/"+ver]
	if build == "" {
		entries, _ := os.ReadDir(e.Paths.Tools)
		prefix := id + "-" + ver + "-build"
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), prefix) {
				build = strings.TrimPrefix(entry.Name(), prefix)
			}
		}
	}
	if build == "" {
		return "", fmt.Errorf("%s %s is not installed", id, ver)
	}
	return filepath.Join(e.Paths.Tools, id+"-"+ver+"-build"+build), nil
}

// InstallTool downloads a CLI tool from the signed catalog using the same
// verified installer as services, without registering a supervisor.
func (e *Engine) InstallTool(ctx context.Context, id, request string) (*installer.Job, error) {
	if id != SQLiteToolID {
		return nil, fmt.Errorf("unknown tool %q", id)
	}
	available, err := e.available(id)
	if err != nil {
		return nil, err
	}
	picked, err := version.ExactOrLatest(request, available)
	if err != nil {
		return nil, err
	}
	art, err := e.artifact(id, picked)
	if err != nil {
		return nil, err
	}
	dest := filepath.Join(e.Paths.Tools, id+"-"+picked+"-build"+art.Build)
	return e.Installer.Start(ctx, installer.Request{
		ID: id + "-" + picked, Package: id, Version: picked, Platform: app.Platform(),
		Artifact: art, Dest: dest, Expected: []string{toolExecutable()},
		ConfigKey: id + "/" + picked,
		OnSuccess: func(string) error {
			return os.WriteFile(filepath.Join(e.Paths.Tools, id+".version"),
				[]byte(picked+"\n"), 0o644)
		},
	})
}

// resolveToolBinary returns the invocation for a catalog-installed CLI.
func (e *Engine) resolveToolBinary(id string) (Invocation, error) {
	ver := e.installedToolVersion(id)
	if ver == "" {
		installed := e.installedToolVersions(id)
		if len(installed) == 0 {
			return Invocation{}, fmt.Errorf("%s is not installed", id)
		}
		ver = installed[0]
	}
	dir, err := e.toolDir(id, ver)
	if err != nil {
		return Invocation{}, err
	}
	bin := filepath.Join(dir, filepath.FromSlash(toolExecutable()))
	if _, err := os.Stat(bin); err != nil {
		return Invocation{}, fmt.Errorf("%s not found: %w", bin, err)
	}
	return Invocation{Path: bin}, nil
}
