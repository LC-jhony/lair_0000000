package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/download"
	"github.com/lair-sh/lair/internal/manifest"
)

func testArchive(t *testing.T) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\n")
	if err := tw.WriteHeader(&tar.Header{Name: "bin/tool", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func TestManagerRun(t *testing.T) {
	root := t.TempDir()
	m := NewManager(filepath.Join(root, "log"), &download.Downloader{}, config.NewStore(filepath.Join(root, "config.json")))
	if _, err := m.Run(context.Background(), RunRequest{}); err == nil {
		t.Fatal("empty run request accepted")
	}
	success := false
	job, err := m.Run(context.Background(), RunRequest{
		ID: "node-nvm-test", Package: "node", Version: "22.16.0",
		Run: func(_ context.Context, log func(string)) error {
			log("Now using node v22.16.0")
			return nil
		},
		OnSuccess: func() error { success = true; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := job.Status(); s.Done {
			if s.Error != "" {
				t.Fatalf("job failed: %s", s.Error)
			}
			if !success || s.Downloaded != s.Total {
				t.Fatalf("success=%v status=%+v", success, s)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("run job did not finish")
}

func TestStartValidationAndSingleJob(t *testing.T) {
	root := t.TempDir()
	m := NewManager(filepath.Join(root, "log"), &download.Downloader{}, config.NewStore(filepath.Join(root, "config.json")))
	if _, err := m.Start(context.Background(), Request{}); err == nil {
		t.Fatal("empty artifact accepted")
	}

	a := manifest.Artifact{URL: "x", SHA256: "abc", Build: "N1"}
	if _, err := m.Start(context.Background(), Request{ID: "x", Artifact: a}); err == nil {
		t.Fatal("short sha accepted")
	}

	// Block the HTTP server so the first job stays "running".
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	archive, sum := testArchive(t)
	_ = archive
	req := Request{
		ID: "php/8.4", Package: "php", Version: "8.4.3", Platform: "linux-x64",
		Artifact: manifest.Artifact{URL: "a.tar.gz", SHA256: sum, Build: "N1"},
		Dest:     filepath.Join(root, "php", "8.4"), Expected: []string{"bin/tool"},
		ConfigKey: "php/8.4.3",
	}
	dl := &download.Downloader{BaseURLs: []string{srv.URL}}
	m2 := NewManager(filepath.Join(root, "log"), dl, config.NewStore(filepath.Join(root, "config.json")))
	j1, err := m2.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m2.Start(context.Background(), req); err == nil {
		t.Fatal("second concurrent job accepted")
	}
	j1.Cancel()
	close(block)
	waitDone(t, j1)
}

func testArchiveNamed(t *testing.T, name string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\n")
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func TestSuccessfulComponentInstall(t *testing.T) {
	root := t.TempDir()
	cli, cliSum := testArchiveNamed(t, "php")
	fpm, fpmSum := testArchiveNamed(t, "php-fpm")
	cliPath := filepath.Join(root, "cli.tar.gz")
	fpmPath := filepath.Join(root, "fpm.tar.gz")
	if err := os.WriteFile(cliPath, cli, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fpmPath, fpm, 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(filepath.Join(root, "log"), &download.Downloader{}, config.NewStore(filepath.Join(root, "config.json")))
	dest := filepath.Join(root, "php", "8.4")
	job, err := m.Start(context.Background(), Request{
		ID: "php-8.4", Package: "php", Version: "8.4.23", Platform: "linux-x64",
		Artifact: manifest.Artifact{
			Build: "N1", URL: "file://" + cliPath, SHA256: cliSum, Size: int64(len(cli)),
			Files: []manifest.FileMap{{From: "php", To: "bin/php"}},
			Components: []manifest.Component{{
				URL: "file://" + fpmPath, SHA256: fpmSum, Size: int64(len(fpm)),
				Files: []manifest.FileMap{{From: "php-fpm", To: "sbin/php-fpm"}},
			}},
		},
		Dest: dest, Expected: []string{"bin/php", "sbin/php-fpm"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, job)
	if s := job.Status(); s.Error != "" || !s.Done || s.Version != "8.4.23" {
		t.Fatalf("job failed: %+v", s)
	}
	for _, rel := range []string{"bin/php", "sbin/php-fpm"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
}

func TestSuccessfulInstall(t *testing.T) {
	root := t.TempDir()
	archive, sum := testArchive(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	defer srv.Close()

	cfgPath := filepath.Join(root, "config.json")
	m := NewManager(filepath.Join(root, "log"),
		&download.Downloader{BaseURLs: []string{srv.URL}},
		config.NewStore(cfgPath))
	dest := filepath.Join(root, "php", "8.4")
	job, err := m.Start(context.Background(), Request{
		ID: "php/8.4", Artifact: manifest.Artifact{URL: "a.tar.gz", SHA256: sum, Build: "N7"},
		Dest: dest, Expected: []string{"bin/tool"}, ConfigKey: "php/8.4.3",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, job)
	if s := job.Status(); s.Error != "" || !s.Done {
		t.Fatalf("job failed: %+v", s)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "build")); string(got) != "N7\n" {
		t.Fatalf("build marker = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "log", logFileName("php/8.4"))); err != nil {
		t.Fatalf("log missing: %v", err)
	}
	c, _ := config.Load(cfgPath)
	if c.Installed["php/8.4.3"] != "N7" {
		t.Fatalf("config not updated: %+v", c.Installed)
	}
	// The job is removed from the running set as soon as it finishes, but its
	// final status must remain queryable so CLI polling never races with it.
	if _, ok := m.Job("php/8.4"); ok {
		t.Fatal("finished job is still running")
	}
	st, ok := m.JobStatus("php/8.4")
	if !ok || !st.Done || st.Error != "" {
		t.Fatalf("finished job status = %+v, ok=%v", st, ok)
	}
}

func TestLogFileNameCollision(t *testing.T) {
	a := logFileName("php/8.4")
	b := logFileName("php-8.4")
	if a == b {
		t.Fatalf("different job ids share a log file: %q", a)
	}
	if logFileName("node-22") != "node-22-install.log" {
		t.Fatal("simple job ids should keep their readable log name")
	}
}

func TestFinishDoesNotDuplicateRecentOrder(t *testing.T) {
	root := t.TempDir()
	m := NewManager(filepath.Join(root, "log"), &download.Downloader{}, config.NewStore(filepath.Join(root, "config.json")))
	job := &Job{status: Status{ID: "same", Done: true}}
	for i := 0; i < 3; i++ {
		m.finish("same", job, nil, true)
	}
	if len(m.recentOrder) != 1 || m.recentOrder[0] != "same" {
		t.Fatalf("recentOrder = %v", m.recentOrder)
	}
}

func TestOnSuccessFailurePropagatesToJobStatus(t *testing.T) {
	root := t.TempDir()
	m := NewManager(filepath.Join(root, "log"), &download.Downloader{}, config.NewStore(filepath.Join(root, "config.json")))
	job, err := m.Run(context.Background(), RunRequest{
		ID:  "fail-setup",
		Run: func(context.Context, func(string)) error { return nil },
		OnSuccess: func() error {
			return fmt.Errorf("setup exploded")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, job)
	st := job.Status()
	if !st.Done || st.Error != "setup exploded" {
		t.Fatalf("OnSuccess failure was not reported: %+v", st)
	}
}

func TestOnSuccessRunsAfterConfigUpdate(t *testing.T) {
	root := t.TempDir()
	archive, sum := testArchive(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	defer srv.Close()
	cfgPath := filepath.Join(root, "config.json")
	m := NewManager(filepath.Join(root, "log"), &download.Downloader{BaseURLs: []string{srv.URL}}, config.NewStore(cfgPath))
	marker := filepath.Join(root, "marker")
	job, err := m.Start(context.Background(), Request{
		ID: "on-success", Artifact: manifest.Artifact{URL: "a.tar.gz", SHA256: sum, Build: "N9"},
		Dest: filepath.Join(root, "dest"), Expected: []string{"bin/tool"}, ConfigKey: "tool/1",
		OnSuccess: func(dest string) error {
			c, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			if c.Installed["tool/1"] != "N9" {
				return fmt.Errorf("OnSuccess ran before the config update: %+v", c.Installed)
			}
			if _, err := os.Stat(filepath.Join(dest, "bin", "tool")); err != nil {
				return fmt.Errorf("OnSuccess ran before the atomic swap: %v", err)
			}
			return os.WriteFile(marker, []byte("ok"), 0o644)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, job)
	if st := job.Status(); st.Error != "" || !st.Done {
		t.Fatalf("job failed: %+v", st)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("OnSuccess was not called: %v", err)
	}
}

func TestNeedsReplacement(t *testing.T) {
	mf := &manifest.Manifest{Packages: map[string]map[string]map[string]manifest.Artifact{
		"postgres": {
			"18": {
				"linux-x64": manifest.Artifact{Build: "N2", SHA256: "abc", Yanked: map[string]string{"N1": "bad"}},
			},
		},
	}}
	if a, ok := NeedsReplacement(mf, "postgres", "18", "linux-x64", "N1"); !ok || a.Build != "N2" {
		t.Fatalf("replacement = %+v %v", a, ok)
	}
	if _, ok := NeedsReplacement(mf, "postgres", "18", "linux-x64", "N2"); ok {
		t.Fatal("current build should not need replacement")
	}
}

func waitDone(t *testing.T, j *Job) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j.Status().Done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not finish")
}
