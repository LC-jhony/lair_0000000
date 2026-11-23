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

// FileMapping moves one regular file from an archive member to a destination
// path inside the install directory. Both paths must be archive-root relative.
type FileMapping struct {
	From string
	To   string
}

// Source is one verified archive to install. When Files is empty the archive
// is unpacked with its original layout; otherwise only the listed members are
// installed, at their mapped destinations.
type Source struct {
	Archive string
	Files   []FileMapping
	// Dir installs an already-extracted directory tree (used by `--from`).
	// It is mutually exclusive with Archive and is copied verbatim, with the
	// same symlink protection as a preserved configuration directory.
	Dir string
	// RawTo installs a verified single-file binary (for example a Meilisearch
	// release) at this exact relative destination, without unpacking it.
	RawTo string
}

// Extract installs a single archive. It is kept for callers that do not need
// multiple sources; Install backs it.
func Extract(archivePath, dest string, expected []string) error {
	return Install([]Source{{Archive: archivePath}}, dest, expected)
}

// Options controls package-specific behaviour of Install. It is a functional
// option so existing three-argument callers keep compiling unchanged.
type Options struct {
	// Preserve lists relative paths owned by the user (php.ini, conf.d, ...).
	// They are copied from the old install into staging before the atomic swap,
	// so an upgrade replaces binaries without destroying local configuration.
	Preserve []string
	// Verify runs against the staging tree after extraction and before the
	// swap. A failure leaves the current installation untouched.
	Verify func(staging string) error
	// AfterExtract can reorganise a merged package (e.g. the Windows PHP
	// extension pack) inside staging before verification and the swap.
	AfterExtract func(staging string, archives []string) error
}

// Option mutates an Options value.
type Option func(*Options)

// WithPreserve asks Install to carry the listed user-owned paths over to the
// new version during an upgrade.
func WithPreserve(paths ...string) Option {
	return func(o *Options) { o.Preserve = append(o.Preserve, paths...) }
}

// WithVerify registers a check that must pass before the atomic swap.
func WithVerify(fn func(staging string) error) Option {
	return func(o *Options) { o.Verify = fn }
}

// WithAfterExtract registers a staging-tree merge step.
func WithAfterExtract(fn func(staging string, archives []string) error) Option {
	return func(o *Options) { o.AfterExtract = fn }
}

// Install unpacks every source into a staging directory next to dest, runs
// Prepare, then atomically swaps staging into dest. dest may already exist
// (an upgrade); the old directory is removed only after the swap succeeds.
func Install(sources []Source, dest string, expected []string, opts ...Option) error {
	if len(sources) == 0 {
		return fmt.Errorf("no sources to install")
	}
	cfg := Options{}
	for _, apply := range opts {
		if apply != nil {
			apply(&cfg)
		}
	}
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".lair-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	for i, src := range sources {
		if src.Dir != "" {
			info, err := os.Stat(src.Dir)
			if err != nil {
				return fmt.Errorf("source %d directory: %w", i, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("source %d: %s is not a directory", i, src.Dir)
			}
			if err := copySourceDir(src.Dir, staging); err != nil {
				return fmt.Errorf("copy source %d: %w", i, err)
			}
			continue
		}
		if src.Archive == "" {
			return fmt.Errorf("source %d has no archive", i)
		}
		if src.RawTo != "" {
			to, err := SafeJoin(staging, src.RawTo)
			if err != nil {
				return err
			}
			if err := copyRawBinary(src.Archive, to); err != nil {
				return fmt.Errorf("install raw binary: %w", err)
			}
			continue
		}
		if len(src.Files) == 0 {
			if err := extractInto(src.Archive, staging); err != nil {
				return fmt.Errorf("extract %d: %w", i, err)
			}
			continue
		}
		unpack, err := os.MkdirTemp(parent, fmt.Sprintf(".lair-source-%d-", i))
		if err != nil {
			return err
		}
		if err := extractInto(src.Archive, unpack); err != nil {
			os.RemoveAll(unpack)
			return fmt.Errorf("extract %d: %w", i, err)
		}
		for _, mapping := range src.Files {
			from, err := SafeJoin(unpack, mapping.From)
			if err != nil {
				os.RemoveAll(unpack)
				return err
			}
			to, err := SafeJoin(staging, mapping.To)
			if err != nil {
				os.RemoveAll(unpack)
				return err
			}
			info, err := os.Lstat(from)
			if err != nil {
				os.RemoveAll(unpack)
				return fmt.Errorf("source member %s: %w", mapping.From, err)
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				os.RemoveAll(unpack)
				return fmt.Errorf("source member %s is not a regular file", mapping.From)
			}
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				os.RemoveAll(unpack)
				return err
			}
			if err := os.Remove(to); err != nil && !os.IsNotExist(err) {
				os.RemoveAll(unpack)
				return err
			}
			if err := os.Rename(from, to); err != nil {
				os.RemoveAll(unpack)
				return fmt.Errorf("place %s: %w", mapping.To, err)
			}
		}
		os.RemoveAll(unpack)
	}

	// A single archive with no explicit mapping may wrap its files in one root
	// directory (official Node and MariaDB tarballs do). Mapped layouts are
	// already normalized by the manifest.
	if len(sources) == 1 && len(sources[0].Files) == 0 && sources[0].RawTo == "" && sources[0].Dir == "" {
		if err := stripSingleRoot(staging, expected); err != nil {
			return fmt.Errorf("normalize layout: %w", err)
		}
	}
	if err := Prepare(staging, expected); err != nil {
		return fmt.Errorf("prepare: %w", err)
	}

	if cfg.AfterExtract != nil {
		if err := cfg.AfterExtract(staging, archives); err != nil {
			return fmt.Errorf("merge staged package: %w", err)
		}
	}

	// Probe the staged binaries before touching the installed version. A
	// package that contains the wrong PHP version (or a broken FPM binary)
	// must never become the active one.
	if cfg.Verify != nil {
		if err := cfg.Verify(staging); err != nil {
			return fmt.Errorf("verify staged package: %w", err)
		}
	}

	// Restore user-owned configuration from the previous install. This runs
	// after verification and before the swap; on failure the old install is
	// still intact. Paths are archive-root relative and SafeJoin-checked.
	if len(cfg.Preserve) > 0 {
		if _, err := os.Lstat(dest); err == nil {
			for _, rel := range cfg.Preserve {
				from, err := SafeJoin(dest, rel)
				if err != nil {
					return err
				}
				to, err := SafeJoin(staging, rel)
				if err != nil {
					return err
				}
				if err := copyPreserved(from, to); err != nil {
					return fmt.Errorf("preserve %s: %w", rel, err)
				}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
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

// copySourceDir copies a --from directory into staging. Official Node trees
// contain relative symlinks (bin/npm -> ../lib/...), so unlike preserved
// configuration they are recreated after checking that the target stays
// inside the source tree.
func copySourceDir(from, to string) error {
	return filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, rel)
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			if filepath.IsAbs(target) {
				return fmt.Errorf("refusing absolute symlink %s", p)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(p), target))
			if inside, err := SafeJoin(from, relOf(from, resolved)); err != nil || inside != resolved {
				return fmt.Errorf("symlink %s escapes the source tree", p)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			return os.Symlink(target, dst)
		}
		if info.IsDir() {
			return os.MkdirAll(dst, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, body, info.Mode().Perm())
	})
}

func relOf(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return ".."
	}
	return rel
}

// copyPreserved copies a user-owned file or directory from the previous
// install into staging. Symlinks are refused: an attacker must not be able to
// redirect a preserved path outside the data root during an upgrade.
func copyPreserved(from, to string) error {
	info, err := os.Lstat(from)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to preserve symlink %s", from)
	}
	if info.IsDir() {
		if err := os.MkdirAll(to, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPreserved(filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	body, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, body, info.Mode().Perm())
}

// copyRawBinary moves a verified download into the staging tree as an
// executable. The bytes were already checked against the signed SHA-256.
func copyRawBinary(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
	// Links are never materialised as links, so a link may point at another
	// link declared later in the archive (MySQL does this for libmysqlclient).
	// Retry unresolved entries until every chain is flattened.
	pending := make(map[string]pendingLink, len(links))
	for _, link := range links {
		pending[link.path] = link
	}
	for len(pending) > 0 {
		progressed := false
		for linkPath, link := range pending {
			base := filepath.Dir(linkPath)
			if link.fromRoot {
				base = dir
			}
			resolved := filepath.Clean(filepath.Join(base, link.target))
			if filepath.IsAbs(link.target) {
				return fmt.Errorf("absolute link target %q in %s", link.target, linkPath)
			}
			rel, err := filepath.Rel(dir, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("link escapes archive: %s -> %s", linkPath, link.target)
			}
			// Wait until the target link has itself been replaced by a regular
			// file with the bytes we can copy.
			if _, waiting := pending[resolved]; waiting {
				continue
			}
			info, err := os.Stat(resolved)
			if err != nil {
				return fmt.Errorf("link target %s: %w", link.target, err)
			}
			if info.IsDir() {
				delete(pending, linkPath)
				progressed = true
				continue
			}
			data, err := os.ReadFile(resolved)
			if err != nil {
				return err
			}
			if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
				return err
			}
			// Copying npm/npx verbatim would break the relative require() calls in
			// npm-cli.js/npx-cli.js. Emit a small wrapper that runs the real CLI
			// with the bundled node binary instead. It is a regular file, so no
			// symlink can escape the install directory.
			if script, ok := nodeCLIWrapper(linkPath, resolved); ok {
				data = []byte(script)
			}
			if err := os.WriteFile(linkPath, data, 0o755); err != nil {
				return err
			}
			delete(pending, linkPath)
			progressed = true
		}
		if !progressed {
			return fmt.Errorf("unresolved archive links: %d", len(pending))
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
