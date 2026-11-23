package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/download"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/node"
	"github.com/lair-sh/lair/internal/php"
	"github.com/lair-sh/lair/internal/version"
)

// localSource validates a --from path and turns it into a download.Source.
// Local builds are never downloaded, but they still go through the same
// staging, verification and atomic swap as catalog artifacts.
func localSource(from string) (download.Source, error) {
	abs, err := filepath.Abs(strings.TrimSpace(from))
	if err != nil {
		return download.Source{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return download.Source{}, fmt.Errorf("--from %s: %w", from, err)
	}
	if info.IsDir() {
		return download.Source{Dir: abs}, nil
	}
	ext := strings.ToLower(filepath.Ext(abs))
	if ext != ".zip" && ext != ".gz" && ext != ".tgz" {
		return download.Source{}, fmt.Errorf("--from %s: expected a directory, .tar.gz or .zip", from)
	}
	return download.Source{Archive: abs}, nil
}

// stagedLocal extracts a local source into a temporary directory under the
// data root (not /tmp, which may be mounted noexec) and returns that directory.
// The caller removes it after probing the version.
func stagedLocal(parentRoot string, src download.Source, expected []string) (string, error) {
	parent, err := os.MkdirTemp(parentRoot, "lair-from-")
	if err != nil {
		return "", err
	}
	target := filepath.Join(parent, "tree")
	if err := download.Install([]download.Source{src}, target, expected); err != nil {
		os.RemoveAll(parent)
		return "", err
	}
	return target, nil
}

// InstallPHPFrom installs a local PHP build (`php install --from`).
func (e *Engine) InstallPHPFrom(ctx context.Context, minor, from string) (*installer.Job, error) {
	src, err := localSource(from)
	if err != nil {
		return nil, err
	}
	staging, err := stagedLocal(e.Paths.Root, src, phpExpected())
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(filepath.Dir(staging))
	bin := "php"
	if strings.HasPrefix(app.Platform(), "windows-") {
		bin = "php.exe"
	}
	out, err := exec.Command(filepath.Join(staging, "bin", bin), "-n", "-v").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run local PHP: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	parsed, parseErr := php.ParseVersionOutput(string(out))
	if parseErr != nil {
		return nil, parseErr
	}
	if minor != "" && parsed.MinorLine() != minor {
		return nil, fmt.Errorf("the build in %s is PHP %s, not PHP %s", from, parsed.Raw, minor)
	}
	picked := parsed.Raw
	dest := filepath.Join(e.Paths.PHP, parsed.MinorLine())
	info := php.Info{Version: picked, Minor: parsed.MinorLine(), ExtPackBuild: e.extPackBuild(parsed.MinorLine())}
	return e.runLocalInstall(ctx, "php-"+parsed.MinorLine(), "php", picked, src, dest, phpExpected(),
		[]string{"php.ini", "conf.d", "conf.d-cli", "conf.d-xdebug", "lair.json"},
		func(stage string) error { return php.Probe(stage, picked) },
		func(dir string) error {
			info.Dir = dir
			if err := php.WriteMarker(info); err != nil {
				return err
			}
			if _, err := php.EnsureINI(info, filepath.Join(e.Paths.Certs, "ca.pem")); err != nil {
				return err
			}
			return e.Config.Update(func(c *config.Config) error {
				if c.Defaults["php"] == "" {
					c.Defaults["php"] = parsed.MinorLine()
				}
				return nil
			})
		})
}

// InstallNodeFrom installs a local Node build (`node install --from`).
func (e *Engine) InstallNodeFrom(ctx context.Context, from string) (*installer.Job, error) {
	src, err := localSource(from)
	if err != nil {
		return nil, err
	}
	staging, err := stagedLocal(e.Paths.Root, src, node.Expected())
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(filepath.Dir(staging))
	bin := "node"
	if strings.HasPrefix(app.Platform(), "windows-") {
		bin = "node.exe"
	}
	out, err := exec.Command(filepath.Join(node.BinDir(staging), bin), "-v").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run local Node: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	picked := strings.TrimSpace(string(out))
	if _, err := version.Parse(picked); err != nil {
		return nil, fmt.Errorf("could not read the local Node version from %s: %s", from, picked)
	}
	major, err := node.MajorOf(picked)
	if err != nil {
		return nil, err
	}
	dest := filepath.Join(e.Paths.Node, picked)
	return e.runLocalInstall(ctx, "node-"+picked, "node", picked, src, dest, node.Expected(), nil, nil,
		func(dir string) error {
			if err := node.EnsureGlobal(e.Paths, major); err != nil {
				return err
			}
			if err := node.RemoveOlderInMajor(e.Paths, picked); err != nil {
				return err
			}
			return e.Config.Update(func(c *config.Config) error {
				if c.Defaults["node"] == "" {
					c.Defaults["node"] = picked
				}
				return nil
			})
		})
}

// runLocalInstall shares the job lifecycle for --from installs. It reuses the
// same installer.Run used for nvm-era jobs, but performs a verified staging
// and atomic swap itself.
func (e *Engine) runLocalInstall(ctx context.Context, id, pkg, ver string, src download.Source, dest string, expected, preserve []string, verify func(string) error, onSuccess func(string) error) (*installer.Job, error) {
	job, err := e.Installer.Run(ctx, installer.RunRequest{
		ID: id, Package: pkg, Version: ver,
		Run: func(ctx context.Context, log func(string)) error {
			opts := []download.Option{}
			if len(preserve) > 0 {
				opts = append(opts, download.WithPreserve(preserve...))
			}
			if verify != nil {
				opts = append(opts, download.WithVerify(verify))
			}
			log("installing local build into staging")
			return download.Install([]download.Source{src}, dest, expected, opts...)
		},
		OnSuccess: func() error { return onSuccess(dest) },
	})
	if err != nil {
		return nil, err
	}
	return job, nil
}
