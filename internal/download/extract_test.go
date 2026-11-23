package download

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TestExtractOfficialStyleArchive reproduces the two properties of official
// Node tarballs that used to break installs: a single top-level directory and
// bin/npm being a symlink. Both must be normalised into a flat layout with
// regular executable files.
func TestExtractOfficialStyleArchive(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name, body string, mode int64, kind byte, link string) {
		h := &tar.Header{Name: name, Mode: mode, Typeflag: kind, Linkname: link, Size: int64(len(body))}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("node-v26.13.1-linux-x64/", "", 0o755, tar.TypeDir, "")
	add("node-v26.13.1-linux-x64/bin/", "", 0o755, tar.TypeDir, "")
	add("node-v26.13.1-linux-x64/bin/node", "node-binary", 0o755, tar.TypeReg, "")
	add("node-v26.13.1-linux-x64/lib/node_modules/npm/bin/npm-cli.js", "#!/usr/bin/env node\n// npm", 0o755, tar.TypeReg, "")
	add("node-v26.13.1-linux-x64/bin/npm", "", 0o755, tar.TypeSymlink, "../lib/node_modules/npm/bin/npm-cli.js")
	add("node-v26.13.1-linux-x64/bin/npx", "", 0o755, tar.TypeSymlink, "../lib/node_modules/npm/bin/npm-cli.js")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	archive := filepath.Join(dir, "node.tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "install")
	if err := Extract(archive, dest, []string{"bin/node", "bin/npm", "bin/npx"}); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	for _, name := range []string{"bin/node", "bin/npm", "bin/npx"} {
		path := filepath.Join(dest, filepath.FromSlash(name))
		info, err := os.Lstat(path)
		if err != nil {
			t.Errorf("%s missing: %v", name, err)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s is still a symlink", name)
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("%s is not executable", name)
		}
	}
	data, err := os.ReadFile(filepath.Join(dest, "bin", "npm"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`exec "$(dirname "$0")/node"`)) ||
		!bytes.Contains(data, []byte(`../lib/node_modules/npm/bin/npm-cli.js`)) {
		t.Fatalf("npm symlink was not replaced by a working wrapper: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dest, "node-v26.13.1-linux-x64")); !os.IsNotExist(err) {
		t.Errorf("top-level prefix was not stripped: %v", err)
	}
}

// TestExtractMariaDBLayout checks the other official layout Lair relies on:
// MariaDB's tarball wraps everything in a version directory but keeps its
// initialisation script under scripts/ rather than bin/.
func TestInstallRawBinary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "meilisearch")
	if err := os.WriteFile(file, []byte("#!/bin/sh\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "services", "meilisearch", "1.57.0-buildN1")
	if err := Install([]Source{{Archive: file, RawTo: "bin/meilisearch"}}, dest, []string{"bin/meilisearch"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "bin", "meilisearch"))
	if err != nil || string(data) != "#!/bin/sh\ntrue\n" {
		t.Fatalf("raw binary = %q, %v", data, err)
	}
	info, err := os.Stat(filepath.Join(dest, "bin", "meilisearch"))
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("raw binary is not executable: %v", err)
	}
}

func TestExtractMariaDBLayout(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"mariadb-11.4.7-linux-systemd-x86_64/bin/mariadbd":               "daemon",
		"mariadb-11.4.7-linux-systemd-x86_64/scripts/mariadb-install-db": "#!/bin/sh\n",
	} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Typeflag: tar.TypeReg, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "mariadb.tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "install")
	if err := Extract(archive, dest, []string{"bin/mariadbd", "scripts/mariadb-install-db"}); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	for _, name := range []string{"bin/mariadbd", "scripts/mariadb-install-db"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(name))); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}
}

// TestExtractRejectsEscapingSymlink makes sure link normalisation cannot be
// used to write outside the staging directory.
func TestExtractRejectsEscapingSymlink(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "bin/node", Mode: 0o755, Typeflag: tar.TypeReg, Size: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("node")); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "bin/npm", Mode: 0o755, Typeflag: tar.TypeSymlink, Linkname: "../../../../etc/passwd"}); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()

	dir := t.TempDir()
	archive := filepath.Join(dir, "bad.tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Extract(archive, filepath.Join(dir, "install"), []string{"bin/node"}); err == nil {
		t.Fatal("escaping symlink was accepted")
	}
}
