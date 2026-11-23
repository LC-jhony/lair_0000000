package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDumpsFlags(t *testing.T) {
	req, err := dumpsFlags([]string{
		"--site", "acme.test", "--type", "queries", "--time", "2026-01-01T00:00:00Z",
		"--request", "GET /users", "--message", "hola mundo", "--token", "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Site != "acme.test" || req.Type != "queries" || req.Request != "GET /users" ||
		req.Message != "hola mundo" || req.Token != "abc" || req.Time == "" {
		t.Fatalf("flags = %+v", req)
	}
	if _, err := dumpsFlags([]string{"--danger", "x"}); err == nil {
		t.Fatal("unknown flag accepted")
	}
	if _, err := dumpsFlags([]string{"--site"}); err == nil {
		t.Fatal("missing flag value accepted")
	}
}

func TestDumpsIngestCLITokenGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LAIR_HOME", home)
	runDir := filepath.Join(home, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	token := "a-secret-token"
	if err := os.WriteFile(filepath.Join(runDir, "dumps.token"), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	args := []string{"dumps", "ingest", "--site", "acme.test", "--type", "queries", "--message", "select 1"}
	if err := run(append(append([]string{}, args...), "--token", "wrong"), &out, &errOut); err == nil {
		t.Fatal("invalid token accepted")
	}
	if _, err := os.Stat(filepath.Join(home, "dumps", "acme.test.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("bad token wrote an event: %v", err)
	}

	out.Reset()
	errOut.Reset()
	if err := run(append(append([]string{}, args...), "--token", token), &out, &errOut); err != nil {
		t.Fatalf("ingest: %v (%s)", err, errOut.String())
	}
	data, err := os.ReadFile(filepath.Join(home, "dumps", "acme.test.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "select 1") || !strings.Contains(string(data), "acme.test") {
		t.Fatalf("jsonl = %s", data)
	}
}
