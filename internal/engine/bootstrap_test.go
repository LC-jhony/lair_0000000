package engine

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/assets"
	"github.com/lair-sh/lair/internal/manifest"
)

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("mirror caído")
}

func bootstrapPaths(t *testing.T) app.Paths {
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
	return paths
}

func signedCatalog(t *testing.T, paths app.Paths, client *http.Client) *manifest.Catalog {
	t.Helper()
	pub, err := assets.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := manifest.NewCatalog(manifest.CatalogConfig{
		PublicKey: pub, Channel: "stable",
		BaseURLs: []string{"https://mirror.invalid/catalog"},
		Root:     paths.Root, HTTP: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, sig := assets.ExampleCatalog()
	if err := cat.Load(data, sig); err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestRefreshCatalogOfflineKeepsLocalCatalog(t *testing.T) {
	paths := bootstrapPaths(t)
	cat := signedCatalog(t, paths, &http.Client{Transport: failingTransport{}})
	e, err := New(paths, cat)
	if err != nil {
		t.Fatal(err)
	}

	text, err := e.RefreshCatalog(context.Background())
	if err != nil {
		t.Fatalf("offline refresh must not fail when a signed catalog is loaded: %v", err)
	}
	if !strings.Contains(text, "mirrors") || !strings.Contains(text, "catálogo firmado local") {
		t.Fatalf("warning = %q", text)
	}
	if cat.Get() == nil {
		t.Fatal("local catalog was dropped")
	}
}

func TestRefreshCatalogWithoutCatalogFails(t *testing.T) {
	paths := bootstrapPaths(t)
	e, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.RefreshCatalog(context.Background()); err == nil {
		t.Fatal("refresh without a catalog must fail")
	}
}
