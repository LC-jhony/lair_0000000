package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/lair-sh/lair/internal/composer"
	"github.com/lair-sh/lair/internal/php"
	"github.com/lair-sh/lair/internal/pki"
	"github.com/lair-sh/lair/internal/shims"
)

var errNoCatalog = errors.New("no catalog loaded; start the daemon or check the network")

// RefreshCatalog refreshes the signed manifest now. It is the implementation
// behind `lair update`. A network failure is not fatal when a valid local
// catalog is already loaded: the signed cache keeps working offline and the
// caller receives an explanatory warning instead of a 500.
func (e *Engine) RefreshCatalog(ctx context.Context) (string, error) {
	if e.Catalog == nil {
		return "", errNoCatalog
	}
	changed, err := e.Catalog.Refresh(ctx)
	if err != nil {
		if e.Catalog.Get() != nil {
			return fmt.Sprintf("No se pudo contactar con los mirrors del catálogo (%v). Se mantiene el catálogo firmado local.", err), nil
		}
		return "", err
	}
	if changed {
		return "Catálogo firmado actualizado.", nil
	}
	return "El catálogo firmado ya está actualizado.", nil
}

// Available lists the non-yanked versions of a package for this platform,
// newest first.
func (e *Engine) Available(pkg string) ([]string, error) { return e.available(pkg) }

// InstallComposer downloads and verifies tools/composer.phar. The URLs can be
// overridden (mirrors, offline tests) with LAIR_COMPOSER_DOWNLOAD_URL and
// LAIR_COMPOSER_SHA256; the SHA-256 check is always enforced. The signed
// catalog supplies the pinned version and checksum by default.
func (e *Engine) InstallComposer(ctx context.Context) (string, error) {
	return e.composerInstaller().Ensure(ctx, e.Paths.Tools)
}

// ComposerUpdate re-downloads and re-verifies composer.phar. It is the
// implementation of `lair composer update` and the desktop "Update Composer"
// action.
func (e *Engine) ComposerUpdate(ctx context.Context) (string, error) {
	return e.composerInstaller().Update(ctx, e.Paths.Tools)
}

// composerInstaller resolves the URL/checksum/version from the signed catalog,
// with the documented offline-test overrides. The checksum check is mandatory.
func (e *Engine) composerInstaller() *composer.Installer {
	inst := &composer.Installer{
		DownloadURL: os.Getenv("LAIR_COMPOSER_DOWNLOAD_URL"),
		ChecksumURL: os.Getenv("LAIR_COMPOSER_CHECKSUM_URL"),
		Expected:    os.Getenv("LAIR_COMPOSER_SHA256"),
	}
	versions, err := e.available("composer")
	if err == nil && len(versions) > 0 {
		if art, aerr := e.artifact("composer", versions[0]); aerr == nil {
			if inst.DownloadURL == "" {
				inst.DownloadURL = art.URL
			}
			if inst.Expected == "" {
				inst.Expected = art.SHA256
			}
			inst.Version = versions[0]
		}
	}
	return inst
}

// ComposerVersion returns the pinned version installed in tools/, if any.
func (e *Engine) ComposerVersion() string {
	return composer.InstalledVersion(e.Paths.Tools)
}

// EnsureShims regenerates the shims and the PATH snippets using this engine's
// data root. Unlike Bootstrap it never touches the network or the CA, so it is
// safe to call after every install from the CLI, the TUI or the desktop app.
func (e *Engine) EnsureShims(lairBinary, home string) (string, error) {
	if err := e.Paths.Ensure(); err != nil {
		return "", err
	}
	if lairBinary == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		lairBinary = exe
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if err := shims.Generate(e.Paths.Bin, lairBinary); err != nil {
		return "", err
	}
	if err := shims.EnsureProfiles(home, e.Paths.Bin); err != nil {
		return "", err
	}
	return e.Paths.Bin, nil
}

// Bootstrap performs the local (unprivileged) part of `lair install`:
// shims, shell profile snippets, local CA and a verified Composer phar. The
// privileged plan (systemd/polkit/resolved/CA trust) is rendered by the CLI
// afterwards.
//
// Shims and PATH snippets are created FIRST and are fatal on failure: without
// them no installed tool is reachable from a shell. Composer and the CA are
// best-effort and reported as warnings, so a blocked network cannot undo an
// otherwise working PHP/Node/service installation.
func (e *Engine) Bootstrap(ctx context.Context, lairBinary, home string) (phar string, warnings []string, err error) {
	if err := e.Paths.Ensure(); err != nil {
		return "", nil, err
	}

	// Essential: a shim is the only way a shell reaches an installed version.
	if lairBinary == "" {
		lairBinary, err = os.Executable()
		if err != nil {
			return "", nil, err
		}
	}
	if err := shims.Generate(e.Paths.Bin, lairBinary); err != nil {
		return "", nil, err
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if err := shims.EnsureProfiles(home, e.Paths.Bin); err != nil {
		return "", nil, err
	}

	// Windows PHP builds need the VC++ runtime. Warn with the official link
	// instead of letting php.exe fail later with a cryptic system dialog.
	if php.VCRedistMissing() {
		warnings = append(warnings, "PHP en Windows requiere el Visual C++ Redistributable x64: "+php.VCRedistURL)
	}

	// Nice to have: secure local HTTPS.
	if _, _, caErr := pki.EnsureCA(e.Paths.Certs); caErr != nil {
		warnings = append(warnings, fmt.Sprintf("no se pudo crear la CA local: %v", caErr))
	}

	// Needs network; a failure must never remove the shims created above.
	// Bound the wait so an offline machine does not hang for minutes.
	composerCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	phar, err = e.InstallComposer(composerCtx)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf(
			"no se pudo instalar composer.phar: %v (vuelve a intentarlo con 'lair install' cuando haya red)", err))
	}
	return phar, warnings, nil
}

// CancelJob cancels a running install if it exists.
func (e *Engine) CancelJob(id string) bool {
	job, ok := e.Installer.Job(id)
	if ok {
		job.Cancel()
	}
	return ok
}
