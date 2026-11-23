package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lair-sh/lair/internal/app"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestRuntimeServesDesktopAPI(t *testing.T) {
	root := t.TempDir()
	t.Setenv(app.EnvHome, root)
	paths, err := app.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	cfg := Defaults(paths, nil)
	cfg.EnableDNS = false
	cfg.EnableWeb = false
	cfg.EnableAPI = false
	cfg.EnableDesktop = true
	cfg.DesktopPort = freePort(t)

	rt, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rt.web.Router != rt.Engine.Web.Router {
		t.Fatal("web router is not shared with the engine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- rt.Run(ctx) }()

	urlPath := filepath.Join(paths.Run, "desktop.url")
	tokenPath := filepath.Join(paths.Run, "desktop.token")
	var apiURL string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(urlPath); err == nil && strings.TrimSpace(string(b)) != "" {
			apiURL = strings.TrimSpace(string(b))
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if apiURL == "" {
		cancel()
		t.Fatalf("desktop.url was not written: %v", <-errCh)
	}
	if strings.Contains(apiURL, "token") {
		t.Fatalf("base URL must not contain a token: %s", apiURL)
	}
	tok, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(tok))
	if token == "" {
		t.Fatal("desktop.token is empty")
	}
	info, err := os.Stat(tokenPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("desktop.token mode = %v, %v", info, err)
	}

	// No web UI: the root path must not serve an HTML page.
	resp, err := http.Get(apiURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET / = %d, want 404 (no web UI)", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, apiURL+"/api", strings.NewReader(`{"action":"status"}`))
	req.Header.Set("X-Lair-Token", token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api = %d: %s", resp.StatusCode, body)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Home string `json:"home"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || env.Data.Home != root {
		t.Fatalf("unexpected status payload: %s", body)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not stop")
	}
	if _, err := os.Stat(urlPath); !os.IsNotExist(err) {
		t.Fatalf("desktop.url not removed on shutdown: %v", err)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("desktop.token not removed on shutdown: %v", err)
	}
}
