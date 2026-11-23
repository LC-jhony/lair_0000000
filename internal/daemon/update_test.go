package daemon

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/assets"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/manifest"
)

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("mirror caído")
}

func updateDispatcher(t *testing.T) *Dispatcher {
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

	pub, err := assets.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := manifest.NewCatalog(manifest.CatalogConfig{
		PublicKey: pub, Channel: "stable",
		BaseURLs: []string{"https://mirror.invalid/catalog"},
		Root:     root, HTTP: &http.Client{Transport: failingTransport{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, sig := assets.ExampleCatalog()
	if err := cat.Load(data, sig); err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(paths, cat)
	if err != nil {
		t.Fatal(err)
	}
	return NewDispatcher(e, nil)
}

func TestUpdateReturnsWarningInsteadOfServerError(t *testing.T) {
	d := updateDispatcher(t)
	var out TextResult
	if err := d.Call(context.Background(), ActionUpdate, nil, &out); err != nil {
		t.Fatalf("offline update with a local catalog must succeed: %v", err)
	}
	if !strings.Contains(out.Text, "mirrors") {
		t.Fatalf("warning = %q", out.Text)
	}
}
