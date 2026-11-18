package download

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// SafeJoin resolves an archive member name under base, refusing absolute
// paths, parent traversal and any path that escapes base after cleaning.
// Backslashes are treated as separators so Windows-style traversal cannot slip
// through on Unix hosts.
func SafeJoin(base, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty archive member name")
	}
	normalized := strings.ReplaceAll(name, `\`, "/")
	if strings.HasPrefix(normalized, "/") || path.IsAbs(normalized) || filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute archive member %q", name)
	}
	cleaned := path.Clean(normalized)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("archive member escapes base: %q", name)
	}
	target := filepath.Join(base, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive member escapes base: %q", name)
	}
	return target, nil
}

// ensureNoSymlinks walks from base to target and rejects any symlink along the
// way, preventing symlink-based escapes even when member names are clean.
func ensureNoSymlinks(base, target string) error {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return err
	}
	cur := base
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			return nil // remaining components do not exist yet
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write through symlink %s", cur)
		}
	}
	return nil
}

// Extract unpacks an archive into a staging directory next to dest, runs
// Prepare, then atomically swaps staging into dest. dest may already exist
// (an upgrade); the old directory is removed only after the swap succeeds.
func Extract(archivePath, dest string, expected []string) error {
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".lair-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	if err := extractInto(archivePath, staging); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	if err := stripSingleRoot(staging, expected); err != nil {
		return fmt.Errorf("normalize layout: %w", err)
	}
	if err := Prepare(staging, expected); err != nil {
		return fmt.Errorf("prepare: %w", err)
	}

	var backup string
	if _, err := os.Lstat(dest); err == nil {
		backup, err = os.MkdirTemp(parent, ".lair-old-")
		if err != nil {
			return err
		}
		os.Remove(backup) // rename needs a free target path
		if err := os.Rename(dest, backup); err != nil {
			return fmt.Errorf("backup existing install: %w", err)
		}
	}
	if err := os.Rename(staging, dest); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
		return fmt.Errorf("install staging: %w", err)
	}
	if backup != "" {
		os.RemoveAll(backup)
	}
	return nil
}

func extractInto(archivePath, dest string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, dest)
	}
	if strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		return extractTarGz(archivePath, dest)
	}
	// Sniff the first bytes when the extension is missing.
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	buf := make([]byte, 4)
	_, _ = io.ReadFull(f, buf)
	f.Close()
	if buf[0] == 'P' && buf[1] == 'K' {
		return extractZip(archivePath, dest)
	}
	if buf[0] == 0x1f && buf[1] == 0x8b {
		return extractTarGz(archivePath, dest)
	}
	return fmt.Errorf("unknown archive format for %s", archivePath)
}

// pendingLink records a symlink or hardlink from an archive. Links are never
// materialised as links: they are replaced by a regular copy of their target so
// no symlink can ever escape the staging directory.
type pendingLink struct {
	path     string // path on disk where the link was declared
	target   string // link target from the archive header
	fromRoot bool   // hardlink targets are archive-root relative
}

func extractTarGz(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var links []pendingLink
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return materializeLinks(dest, links)
		}
		if err != nil {
			return err
		}
		target, err := SafeJoin(dest, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeSymlink:
			links = append(links, pendingLink{path: target, target: hdr.Linkname})
			continue
		case tar.TypeLink:
			links = append(links, pendingLink{path: target, target: hdr.Linkname, fromRoot: true})
			continue
		case tar.TypeDir:
			if err := checkMemberType(hdr.Typeflag, hdr.Name); err != nil {
				return err
			}
			if err := ensureNoSymlinks(dest, target); err != nil {
				return err
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		case tar.TypeReg, tar.TypeRegA:
			// regular file below
		default:
			// devices, FIFOs and every other exotic type are refused.
			return fmt.Errorf("unsupported archive member type %d for %q", hdr.Typeflag, hdr.Name)
		}
		if err := ensureNoSymlinks(dest, target); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}

func checkMemberType(flag byte, name string) error {
	switch flag {
	case tar.TypeReg, tar.TypeRegA, tar.TypeDir:
		return nil
	default:
		return fmt.Errorf("unsupported archive member type %d for %q", flag, name)
	}
}

// stripSingleRoot flattens archives that wrap everything in one directory
// (official Node and MariaDB tarballs do). It is a no-op when an expected path
// is already at the root or when the archive has a different layout.
func stripSingleRoot(dir string, expected []string) error {
	for _, rel := range expected {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel))); err == nil {
			return nil
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return nil
	}
	root := filepath.Join(dir, entries[0].Name())
	tmp, err := os.MkdirTemp(dir, ".lair-flatten-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	children, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := os.Rename(filepath.Join(root, child.Name()), filepath.Join(tmp, child.Name())); err != nil {
			return err
		}
	}
	if err := os.Remove(root); err != nil {
		return err
	}
	children, err = os.ReadDir(tmp)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := os.Rename(filepath.Join(tmp, child.Name()), filepath.Join(dir, child.Name())); err != nil {
			return err
		}
	}
	return nil
}

// materializeLinks replaces archive links with regular files containing the
// bytes of their target. Every target is resolved and checked to stay inside
// the staging directory first, so this cannot be used to write outside it.
func materializeLinks(dir string, links []pendingLink) error {
	for _, link := range links {
		base := filepath.Dir(link.path)
		if link.fromRoot {
			base = dir
		}
		resolved := filepath.Clean(filepath.Join(base, link.target))
		if filepath.IsAbs(link.target) {
			return fmt.Errorf("absolute link target %q in %s", link.target, link.path)
		}
		rel, err := filepath.Rel(dir, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("link escapes archive: %s -> %s", link.path, link.target)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return fmt.Errorf("link target %s: %w", link.target, err)
		}
		if info.IsDir() {
			continue
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return err
		}
		if err := os.Remove(link.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(link.path), 0o755); err != nil {
			return err
		}
		// Copying npm/npx verbatim would break the relative require() calls in
		// npm-cli.js/npx-cli.js. Emit a small wrapper that runs the real CLI
		// with the bundled node binary instead. It is a regular file, so no
		// symlink can escape the install directory.
		if script, ok := nodeCLIWrapper(link.path, resolved); ok {
			data = []byte(script)
		}
		if err := os.WriteFile(link.path, data, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// nodeCLIWrapper generates a relocatable shell wrapper for the npm/npx
// symlinks shipped in official Node tarballs. The returned path to the JS CLI
// is relative to bin/, so moving the installation does not break it.
func nodeCLIWrapper(linkPath, resolved string) (string, bool) {
	if filepath.Base(filepath.Dir(linkPath)) != "bin" {
		return "", false
	}
	switch filepath.Base(linkPath) {
	case "npm", "npx":
	default:
		return "", false
	}
	if !strings.EqualFold(filepath.Ext(resolved), ".js") {
		return "", false
	}
	// materializeLinks already proved that resolved is inside the staging
	// directory; a leading "../" here is expected because the CLI lives under
	// lib/node_modules, one level above bin/.
	rel, err := filepath.Rel(filepath.Dir(linkPath), resolved)
	if err != nil {
		return "", false
	}
	return `#!/bin/sh
# Generated by Lair: wrapper for the npm/npx symlink in the official Node tarball.
exec "$(dirname "$0")/node" "$(dirname "$0")/` + filepath.ToSlash(rel) + `" "$@"` + "\n", true
}

func extractZip(archivePath, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, entry := range zr.File {
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in zip: %s", entry.Name)
		}
		target, err := SafeJoin(dest, entry.Name)
		if err != nil {
			return err
		}
		if err := ensureNoSymlinks(dest, target); err != nil {
			return err
		}
		if entry.FileInfo().IsDir() || strings.HasSuffix(entry.Name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
