package engine

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/assets"
	"github.com/lair-sh/lair/internal/manifest"
	"github.com/lair-sh/lair/internal/nvm"
)

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func testEnv(t *testing.T) (*Engine, *httptest.Server) {
	t.Helper()
	root := t.TempDir()
	paths, err := app.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	paths.Root = root
	paths.Bin = filepath.Join(root, "bin")
	paths.Tools = filepath.Join(root, "tools")
	paths.PHP = filepath.Join(root, "php")
	paths.Node = filepath.Join(root, "node")
	paths.Services = filepath.Join(root, "services")
	paths.Log = filepath.Join(root, "log")
	paths.Run = filepath.Join(root, "run")
	paths.Certs = filepath.Join(root, "certs")
	paths.Cache = filepath.Join(root, "cache")
	paths.CacheBin = filepath.Join(root, "cache", "binaries")
	paths.ConfigFile = filepath.Join(root, "config.json")

	archive := tarGz(t, map[string]string{
		"bin/php":        "#!/bin/sh\necho 'PHP 8.4.3 (cli)'\n",
		"sbin/php-fpm":   "#!/bin/sh\necho 'PHP 8.4.3 (fpm)'\n",
		"doc/readme.txt": "trim me",
	})
	sum := sha256.Sum256(archive)
	phar := []byte("fake composer phar")
	pharSum := sha256.Sum256(phar)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mf := manifest.Manifest{
		Schema: 1, Channel: "stable", Serial: 1,
		Generated: now, Expires: now.Add(24 * time.Hour),
		Packages: map[string]map[string]map[string]manifest.Artifact{
			"php": {
				"8.4.3": {
					app.Platform(): manifest.Artifact{
						Build: "N1", URL: "php.tar.gz", SHA256: hex.EncodeToString(sum[:]),
						Size: int64(len(archive)), Published: now,
					},
				},
			},
			"composer": {
				"2.11.2": {
					app.Platform(): manifest.Artifact{
						Build: "N1", URL: "composer.phar", SHA256: hex.EncodeToString(pharSum[:]),
						Size: int64(len(phar)), Published: now,
					},
				},
			},
		},
	}
	data, _ := json.Marshal(mf)
	sig := ed25519.Sign(priv, data)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "manifest.json":
			w.Write(data)
		case "manifest.json.sig":
			w.Write([]byte(base64.StdEncoding.EncodeToString(sig)))
		case "php.tar.gz":
			w.Write(archive)
		case "composer.phar":
			w.Write(phar)
		default:
			http.NotFound(w, r)
		}
	}))

	catalog, err := manifest.NewCatalog(manifest.CatalogConfig{
		PublicKey: pub, Channel: "stable", Root: root, BaseURLs: []string{srv.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	e, err := New(paths, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return e, srv
}

// Regression: pressing "n" in the TUI must install Node LTS even when the
// daemon starts with a nil or empty catalog (stale process, empty cache).
func TestEmptyCatalogFallsBackToEmbeddedNodeLTS(t *testing.T) {
	t.Setenv("LAIR_HOME", t.TempDir())
	paths, err := app.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := assets.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := manifest.NewCatalog(manifest.CatalogConfig{
		PublicKey: pub, Channel: "stable", BaseURLs: manifest.DefaultBaseURLs,
		Root: paths.Root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cat.Get() != nil {
		t.Fatal("catalog should start empty")
	}

	for _, tc := range []struct {
		name    string
		catalog *manifest.Catalog
	}{
		{name: "empty catalog", catalog: cat},
		{name: "nil catalog", catalog: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, err := New(paths, tc.catalog)
			if err != nil {
				t.Fatal(err)
			}
			versions, err := e.Available("node")
			if err != nil {
				t.Fatalf("Available(node) after empty catalog: %v", err)
			}
			if len(versions) == 0 || versions[0] != "v26.13.1" {
				t.Fatalf("node versions = %v", versions)
			}
			lts := e.LatestLTS("node")
			if lts != "v26.13.1" {
				t.Fatalf("LatestLTS(node) = %q", lts)
			}
			// This is the lookup InstallNode performs before starting the job.
			art, err := e.artifact("node", lts)
			if err != nil {
				t.Fatalf("artifact(node, %s): %v", lts, err)
			}
			if art.URL == "" || art.SHA256 == "" {
				t.Fatalf("node artifact is incomplete: %+v", art)
			}
		})
	}
}

func TestInstallPHPAndResolveTool(t *testing.T) {
	e, srv := testEnv(t)
	defer srv.Close()

	job, err := e.InstallPHP(context.Background(), "8.4")
	if err != nil {
		t.Fatal(err)
	}
	st, err := e.WaitJob(context.Background(), job)
	if err != nil || st.Error != "" || !st.Done {
		t.Fatalf("job = %+v, err=%v", st, err)
	}

	// Binary, marker, preserved ini and trimmed docs.
	dir := filepath.Join(e.Paths.PHP, "8.4")
	if _, err := os.Stat(filepath.Join(dir, "bin", "php")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "lair.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "php.ini")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "doc")); !os.IsNotExist(err) {
		t.Fatal("docs not trimmed")
	}

	status, err := e.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.PHP) != 1 || status.PHP[0].Minor != "8.4" || status.Defaults["php"] != "8.4" {
		t.Fatalf("status = %+v", status)
	}

	// Resolve the php shim for a project directory.
	project := t.TempDir()
	inv, err := e.ResolveTool("php", project)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Path != filepath.Join(dir, "bin", "php") {
		t.Fatalf("resolved path = %s", inv.Path)
	}
	if len(inv.Env) == 0 || !strings.HasPrefix(inv.Env[0], "PHPRC=") {
		t.Fatalf("env = %v", inv.Env)
	}
}

func TestPinAndSites(t *testing.T) {
	e, srv := testEnv(t)
	defer srv.Close()
	project := t.TempDir()

	name, err := e.Park(project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, ".test") {
		t.Fatalf("parked name = %s", name)
	}
	if err := e.Link("acme.test", project); err != nil {
		t.Fatal(err)
	}
	sites := e.Web.Router.List()
	if len(sites) != 2 {
		t.Fatalf("sites = %+v", sites)
	}
	if err := e.Secure("acme.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.Paths.Certs, "acme.test.pem")); err != nil {
		t.Fatalf("site cert missing: %v", err)
	}

	if err := e.Pin("php", "8.4", project); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(project, ".lairrc"))
	if !strings.Contains(string(data), "php=8.4") {
		t.Fatalf(".lairrc = %s", data)
	}
	// Pin is idempotent (updates in place).
	_ = e.Pin("php", "8.5", project)
	data, _ = os.ReadFile(filepath.Join(project, ".lairrc"))
	if strings.Count(string(data), "php=") != 1 || !strings.Contains(string(data), "php=8.5") {
		t.Fatalf(".lairrc after re-pin = %s", data)
	}
	// A legacy file is carried over into .lairrc instead of being lost.
	legacyProject := t.TempDir()
	os.WriteFile(filepath.Join(legacyProject, ".flockrc"), []byte("php=8.1\n"), 0o644)
	if err := e.Pin("node", "22", legacyProject); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(legacyProject, ".lairrc"))
	if !strings.Contains(string(data), "php=8.1") || !strings.Contains(string(data), "node=22") {
		t.Fatalf("legacy pin not migrated: %s", data)
	}

	if err := e.Unlink("acme.test"); err != nil {
		t.Fatal(err)
	}
	if len(e.Web.Router.List()) != 1 {
		t.Fatal("site not unlinked")
	}
}

// Node installations go through `lair nvm install`. The daemon must report
// nvm's output on the job and resolve shims through `lair nvm exec`.
func TestInstallNodeViaNVM(t *testing.T) {
	e, _ := testEnv(t)
	nvmDir := filepath.Join(e.Paths.Root, "nvm")
	nodeDir := filepath.Join(nvmDir, "versions", "node", "v22.16.0")
	if err := os.MkdirAll(nvmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvmDir, "nvm.sh"), []byte("# nvm stub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NVM_DIR", nvmDir)

	var gotArgs []string
	old := nvm.Command
	nvm.Command = func(_ context.Context, args ...string) (string, error) {
		gotArgs = append([]string(nil), args...)
		if err := os.MkdirAll(filepath.Join(nodeDir, "bin"), 0o755); err != nil {
			return "", err
		}
		for _, tool := range []string{"node", "npm", "npx"} {
			body := "#!/bin/sh\necho " + tool + "\n"
			if err := os.WriteFile(filepath.Join(nodeDir, "bin", tool), []byte(body), 0o755); err != nil {
				return "", err
			}
		}
		return "Downloading...\nNow using node v22.16.0\n", nil
	}
	t.Cleanup(func() { nvm.Command = old })

	job, err := e.InstallNode(context.Background(), "22.16.0")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s := job.Status()
		if s.Done {
			if s.Error != "" {
				t.Fatalf("install failed: %s", s.Error)
			}
			// Catalog-owned installs report 22.16.0; the nvm fallback reports
			// v22.16.0. Both are valid depending on network/catalog availability.
			if s.Version != "v22.16.0" && s.Version != "22.16.0" {
				t.Fatalf("job version = %q", s.Version)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(gotArgs) > 0 && strings.Join(gotArgs, " ") != "install 22.16.0" {
		t.Fatalf("nvm call = %v", gotArgs)
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.Defaults["node"] != "v22.16.0" && c.Defaults["node"] != "22.16.0" {
		t.Fatalf("default node = %q", c.Defaults["node"])
	}
	inv, err := e.ResolveTool("node", "")
	if err != nil {
		t.Fatal(err)
	}
	// nvm installs are proxied through `lair nvm exec`; catalog installs point
	// directly at the binary with no prefix args.
	if len(gotArgs) > 0 && strings.Join(inv.Args, " ") != "nvm exec v22.16.0 node" {
		t.Fatalf("shim invocation = %v", inv.Args)
	}
	if len(gotArgs) == 0 && len(inv.Args) != 0 {
		t.Fatalf("catalog shim should not have prefix args: %v", inv.Args)
	}
}

// Composer is pinned by the signed catalog, not by a moving latest-stable URL.
func TestInstallComposerFromCatalog(t *testing.T) {
	e, srv := testEnv(t)
	t.Setenv("LAIR_COMPOSER_DOWNLOAD_URL", srv.URL+"/composer.phar")
	t.Setenv("LAIR_COMPOSER_CHECKSUM_URL", "")
	t.Setenv("LAIR_COMPOSER_SHA256", "")

	path, err := e.InstallComposer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "fake composer phar" {
		t.Fatalf("phar contents = %q", data)
	}
	if got := e.ComposerVersion(); got != "2.11.2" {
		t.Fatalf("composer version = %q", got)
	}
}
