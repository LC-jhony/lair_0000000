package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lair-sh/lair/internal/shims"
)

func TestUninstallRemovesEverythingLairInstalled(t *testing.T) {
	paths := testPaths(t)
	home := t.TempDir()
	// Uninstall deliberately uses os.UserHomeDir(), not the payload value.
	t.Setenv("HOME", home)

	dirs := []string{
		paths.Bin,
		paths.Tools,
		paths.PHP,
		paths.Node,
		paths.Cache,
		paths.Certs,
		paths.Run,
		paths.Log,
		filepath.Join(paths.Services, "mariadb", "11.8-buildN1"),
		filepath.Join(paths.Services, "mariadb", "data", "11.8"),
		filepath.Join(paths.Services, "postgres", "17-buildN2"),
		filepath.Join(paths.Root, "nginx"),
		filepath.Join(paths.Root, "valet"),
		filepath.Join(paths.Root, "mailcatcher"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{
		filepath.Join(paths.Tools, "composer.phar"),
		filepath.Join(paths.PHP, "8.4", "lair.json"),
		filepath.Join(paths.Node, "22.11.0", "bin", "node"),
		filepath.Join(paths.Services, "mariadb", "11.8-buildN1", "bin", "mariadbd"),
		filepath.Join(paths.Services, "mariadb", "data", "11.8", "ibdata1"),
		filepath.Join(paths.ConfigFile),
		filepath.Join(paths.Root, "revert-privileged.sh"),
		filepath.Join(paths.Root, "trust-ca.sh"),
		filepath.Join(paths.Root, "trust-ca-user.sh"),
		filepath.Join(home, ".bashrc"),
	}
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(shims.MarkBegin+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	eng, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Uninstall(home, paths.Root, false); err != nil {
		t.Fatal(err)
	}

	// Everything Lair installed must be gone.
	removed := []string{
		paths.Bin, paths.Tools, paths.PHP, paths.Node, paths.Cache, paths.Certs,
		paths.Run, paths.Log, paths.ConfigFile,
		filepath.Join(paths.Services, "mariadb", "11.8-buildN1"),
		filepath.Join(paths.Services, "postgres", "17-buildN2"),
		filepath.Join(paths.Root, "nginx"),
		filepath.Join(paths.Root, "valet"),
		filepath.Join(paths.Root, "mailcatcher"),
	}
	for _, path := range removed {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("still exists after uninstall: %s (%v)", path, err)
		}
	}

	// Databases and the scripts needed to finish privileged cleanup survive.
	kept := []string{
		filepath.Join(paths.Services, "mariadb", "data", "11.8", "ibdata1"),
		filepath.Join(paths.Root, "revert-privileged.sh"),
		filepath.Join(paths.Root, "trust-ca.sh"),
		filepath.Join(paths.Root, "trust-ca-user.sh"),
	}
	for _, path := range kept {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("unexpectedly removed: %s (%v)", path, err)
		}
	}
	body, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if string(body) != "" {
		t.Errorf("PATH snippet was not removed from .bashrc: %q", body)
	}
}

func TestUninstallIgnoresHostilePayloadPaths(t *testing.T) {
	paths := testPaths(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(paths.Bin), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A daemon client may send any home/data_dir it wants. Neither may be used
	// as a deletion target: only e.Paths.Root and the real HOME are trusted.
	victim := t.TempDir()
	marker := filepath.Join(victim, "keep-me")
	if err := os.WriteFile(marker, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := eng.Uninstall(victim, victim, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("hostile data_dir was deleted: %v", err)
	}
	if _, err := os.Stat(paths.Bin); !os.IsNotExist(err) {
		t.Fatalf("real data root was not cleaned: %v", err)
	}
}

func TestUninstallDeleteDataIgnoresPayloadDataDir(t *testing.T) {
	paths := testPaths(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(paths.Services, "mariadb", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	victim := t.TempDir()
	marker := filepath.Join(victim, "keep-me")
	os.WriteFile(marker, []byte("data"), 0o644)
	if err := eng.Uninstall(victim, victim, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("hostile data_dir was deleted with --delete-data: %v", err)
	}
	if _, err := os.Stat(paths.Root); !os.IsNotExist(err) {
		t.Fatalf("real data root survived --delete-data: %v", err)
	}
}

func TestUninstallDeleteDataRemovesRoot(t *testing.T) {
	paths := testPaths(t)
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(paths.Services, "mariadb", "data", "11.8"), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Uninstall(t.TempDir(), paths.Root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Root); !os.IsNotExist(err) {
		t.Fatalf("root still exists after --delete-data: %v", err)
	}
}
