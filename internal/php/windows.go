package php

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// MergeExtensionPack reorganises a Windows PHP build plus its extension pack
// inside the staging tree, before the atomic swap:
//
//   - ext/*.dll stay under ext/ (EnsureAppINI emits extension= lines for them)
//   - DLLs that those extensions load (ImageMagick and friends) are placed
//     next to php.exe, where the Windows loader can find them
//   - licenses end up in LICENSES/
//   - extras/ssl/openssl.cnf is copied to bin/openssl.cnf so
//     openssl_pkey_new() works (used by `passport:keys`)
//
// The pack is a separate verified artifact; by the time this runs its SHA-256
// has already been checked. No file is fetched or written outside staging.
func MergeExtensionPack(staging string) error {
	extDir := filepath.Join(staging, "ext")
	binDir := filepath.Join(staging, "bin")
	licDir := filepath.Join(staging, "LICENSES")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(extDir); err == nil && info.IsDir() {
		// Already merged; only the helper libraries and openssl.cnf may be new.
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Move shared libraries next to the executables. The pack may put them at
	// the archive root, in lib/, or in bin/.
	for _, dir := range []string{staging, filepath.Join(staging, "lib")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".dll") {
				continue
			}
			src := filepath.Join(dir, entry.Name())
			dst := filepath.Join(binDir, entry.Name())
			if samePath(src, dst) {
				continue
			}
			if err := os.Rename(src, dst); err != nil {
				return fmt.Errorf("place %s: %w", entry.Name(), err)
			}
		}
	}

	// Licenses belong together, even when the pack uses a different case.
	for _, name := range []string{"licenses", "LICENSE", "LICENSE.txt"} {
		src := filepath.Join(staging, name)
		if info, err := os.Stat(src); err != nil || !info.IsDir() {
			continue
		}
		if err := os.MkdirAll(licDir, 0o755); err != nil {
			return err
		}
		if err := moveTree(src, licDir); err != nil {
			return err
		}
	}

	// PHP on Windows does not use the system CA store for openssl_pkey_new.
	// The official zip keeps a usable config under extras/ssl/.
	cnf := filepath.Join(staging, "extras", "ssl", "openssl.cnf")
	if info, err := os.Stat(cnf); err == nil && !info.IsDir() {
		body, err := os.ReadFile(cnf)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(binDir, "openssl.cnf"), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func samePath(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return a == b
}

func moveTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(to, info.Mode().Perm())
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		return os.Rename(p, to)
	})
}

// VCRedistMissing reports whether the Microsoft Visual C++ Redistributable
// x64 is absent on Windows. PHP's official builds link against it; without it
// php.exe fails to start with a cryptic dialog, so the app must warn first.
func VCRedistMissing() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	// The redistributable registers its x64 runtime under this key. `reg.exe`
	// is always present on Windows and needs no privileges to query.
	return exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\X64`).Run() != nil
}

// VCRedistURL is the official, current x64 redistributable download.
const VCRedistURL = "https://aka.ms/vs/17/release/vc_redist.x64.exe"
