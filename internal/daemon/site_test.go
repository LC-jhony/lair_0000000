package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiteActionOverDispatcher(t *testing.T) {
	d, _ := testDispatcher(t)
	ctx := context.Background()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("APP_URL=http://blog.test:8080\nDB_PASSWORD=hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.Call(ctx, ActionLink, LinkRequest{Name: "blog.test", Path: project}, nil); err != nil {
		t.Fatal(err)
	}

	var res SiteResult
	if err := d.Call(ctx, ActionSite, SiteRequest{Op: "detail", Name: "blog.test"}, &res); err != nil {
		t.Fatal(err)
	}
	if res.Detail == nil || res.Detail.Name != "blog.test" || res.Detail.Path != project {
		t.Fatalf("detail = %+v", res.Detail)
	}

	if err := d.Call(ctx, ActionSite, SiteRequest{Op: "env", Name: "blog.test"}, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Env) != 2 {
		t.Fatalf("env entries = %+v", res.Env)
	}
	for _, entry := range res.Env {
		if entry.Key == "DB_PASSWORD" && (entry.Value != "" || !entry.Secret || !entry.Set) {
			t.Fatalf("password was not masked: %+v", entry)
		}
	}
	encoded, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "hunter2") {
		t.Fatalf("secret leaked in wire result: %s", encoded)
	}
}

func TestUnlinkRequestDeleteFiles(t *testing.T) {
	d, _ := testDispatcher(t)
	ctx := context.Background()

	keep := t.TempDir()
	if err := d.Call(ctx, ActionLink, LinkRequest{Name: "keep.test", Path: keep}, nil); err != nil {
		t.Fatal(err)
	}
	// Legacy payload shape ({name}) keeps the project files.
	if err := d.Call(ctx, ActionUnlink, LogsRequest{Name: "keep.test"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("legacy unlink deleted files: %v", err)
	}

	remove := t.TempDir()
	if err := os.WriteFile(filepath.Join(remove, "index.php"), []byte("<?php\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.Call(ctx, ActionLink, LinkRequest{Name: "remove.test", Path: remove}, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Call(ctx, ActionUnlink, UnlinkRequest{Name: "remove.test", DeleteFiles: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(remove); !os.IsNotExist(err) {
		t.Fatalf("delete_files did not remove the project: %v", err)
	}
}
