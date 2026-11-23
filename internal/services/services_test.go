package services

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/app"
)

func testPaths(t *testing.T) app.Paths {
	t.Helper()
	root := t.TempDir()
	return app.Paths{
		Root: root, Services: filepath.Join(root, "services"),
		Run: filepath.Join(root, "run"), Log: filepath.Join(root, "log"),
	}
}

func TestSpecsBindLoopback(t *testing.T) {
	p := testPaths(t)
	for _, id := range IDs() {
		s, err := Build(p, id, "8.4", DefaultPort(id), "N1")
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		joined := strings.Join(s.Args, " ")
		if !strings.Contains(joined, "127.0.0.1") {
			t.Errorf("%s does not bind loopback: %s", id, joined)
		}
		if s.Executable == "" || s.DataDir == "" || s.EnvSnippet == "" {
			t.Errorf("%s has incomplete spec: %+v", id, s)
		}
	}
}

func TestPostgresSpec(t *testing.T) {
	p := testPaths(t)
	s, err := Build(p, "postgres", "18.1", 5432, "N3")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(s.Args, " ")
	if !strings.Contains(joined, "listen_addresses=127.0.0.1") || !strings.Contains(joined, "-p 5432") {
		t.Fatalf("postgres args = %s", joined)
	}
	if len(s.ClientTools) != 5 {
		t.Fatalf("client tools = %v", s.ClientTools)
	}
	if !strings.Contains(s.EnvSnippet, "DB_CONNECTION=pgsql") || !strings.Contains(s.EnvSnippet, "DB_PORT=5432") {
		t.Fatalf("env snippet = %s", s.EnvSnippet)
	}
	// pg_ctl stop uses fast mode.
	if len(s.StopCmd) < 5 || s.StopCmd[1] != "stop" || s.StopCmd[3] != "fast" {
		t.Fatalf("stop cmd = %v", s.StopCmd)
	}

	cmd, _, err := InitCommand(p, "postgres", "18.1", "N3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(cmd, " "), "--auth=trust") {
		t.Fatalf("initdb args = %v", cmd)
	}
}

func TestMySQLAndMariaDBSpecs(t *testing.T) {
	p := testPaths(t)
	my, err := Build(p, "mysql", "8.4", 3306, "N1")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(my.Args, " ")
	for _, want := range []string{"--defaults-file=", "--bind-address=127.0.0.1", "--port=3306", "--socket=", "--mysqlx=OFF"} {
		if !strings.Contains(joined, want) {
			t.Errorf("mysql args missing %s: %s", want, joined)
		}
	}
	if !strings.HasSuffix(my.SocketPath, "mysql.sock") {
		t.Fatalf("socket = %s", my.SocketPath)
	}

	maria, _ := Build(p, "mariadb", "11.8", 3307, "N1")
	cmd, workDir, err := InitCommand(p, "mariadb", "11.8", "N1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(cmd, " "), "--basedir=.") || !strings.Contains(strings.Join(cmd, " "), "--skip-test-db") {
		t.Fatalf("mariadb init = %v", cmd)
	}
	if workDir != filepath.Join(p.Services, "mariadb", "11.8-buildN1") {
		t.Fatalf("workdir = %s", workDir)
	}
	if !strings.Contains(maria.EnvSnippet, "DB_CONNECTION=mariadb") {
		t.Fatalf("mariadb env = %s", maria.EnvSnippet)
	}
}

func TestRedisAndValkeySpecs(t *testing.T) {
	p := testPaths(t)
	for _, id := range []string{"redis", "valkey"} {
		s, err := Build(p, id, "9.2.0", 6379, "N1")
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		joined := strings.Join(s.Args, " ")
		for _, want := range []string{"--port 6379", "--bind 127.0.0.1", "--dir ", "--protected-mode yes", "--daemonize no"} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s args missing %s: %s", id, want, joined)
			}
		}
		if s.StopSignal != "SIGTERM" || len(s.ClientTools) != 1 {
			t.Errorf("%s stop/tools = %q %v", id, s.StopSignal, s.ClientTools)
		}
		if !strings.Contains(s.EnvSnippet, "REDIS_PORT=6379") {
			t.Errorf("%s env = %s", id, s.EnvSnippet)
		}
		cmd, _, err := InitCommand(p, id, "9.2.0", "N1")
		if err != nil || len(cmd) != 0 {
			t.Errorf("%s init = %v, %v", id, cmd, err)
		}
	}
	valkey, _ := Build(p, "valkey", "9.2.0", 6379, "N1")
	if !strings.HasSuffix(valkey.Executable, "bin/valkey-server") || valkey.ClientTools[0] != "valkey-cli" {
		t.Fatalf("valkey binaries = %s %v", valkey.Executable, valkey.ClientTools)
	}
	if DefaultPort("valkey") != 6379 {
		t.Fatal("valkey must default to 6379")
	}
}

func TestSearchAndStorageSpecs(t *testing.T) {
	p := testPaths(t)
	for _, tc := range []struct {
		id, binary string
		port       int
		env        string
	}{
		{"meilisearch", "meilisearch", 7700, "MEILISEARCH_HOST=http://127.0.0.1:7700"},
		{"typesense", "typesense-server", 8108, "TYPESENSE_API_KEY=lair"},
		{"rustfs", "rustfs", 9000, "AWS_ENDPOINT=http://127.0.0.1:9000"},
	} {
		s, err := Build(p, tc.id, "1.0.0", tc.port, "N1")
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if !strings.HasSuffix(s.Executable, "bin/"+tc.binary) {
			t.Errorf("%s executable = %s", tc.id, s.Executable)
		}
		if !strings.Contains(strings.Join(s.Args, " "), "127.0.0.1") {
			t.Errorf("%s does not bind loopback: %v", tc.id, s.Args)
		}
		if !strings.Contains(s.EnvSnippet, tc.env) {
			t.Errorf("%s env = %s", tc.id, s.EnvSnippet)
		}
		if s.StopSignal != "SIGTERM" {
			t.Errorf("%s stop signal = %q", tc.id, s.StopSignal)
		}
		if _, _, err := InitCommand(p, tc.id, "1.0.0", "N1"); err != nil {
			t.Errorf("%s init: %v", tc.id, err)
		}
		if DefaultPort(tc.id) != tc.port {
			t.Errorf("%s default port = %d", tc.id, DefaultPort(tc.id))
		}
	}
	rust, _ := Build(p, "rustfs", "1.0.6", 9000, "N1")
	if !strings.Contains(strings.Join(rust.Args, " "), "--console-address 127.0.0.1:9001") {
		t.Fatalf("rustfs console = %v", rust.Args)
	}
	if ok, _ := Availability("typesense", "windows-x64"); ok {
		t.Fatal("typesense must be unavailable on Windows")
	}
	if ok, _ := Availability("meilisearch", "linux-x64"); !ok {
		t.Fatal("meilisearch must be available on Linux")
	}
}

func TestDataSeparationByMajor(t *testing.T) {
	p := testPaths(t)
	a := DataPath(p, "postgres", "18.1")
	b := DataPath(p, "postgres", "18.5")
	c := DataPath(p, "postgres", "17.9")
	if a != b {
		t.Fatalf("same major should share data dir: %s != %s", a, b)
	}
	if a == c {
		t.Fatal("different major must have separate data dir")
	}
	m1 := DataPath(p, "mysql", "8.4")
	m2 := DataPath(p, "mysql", "8.0")
	if m1 == m2 {
		t.Fatal("mysql data dirs should differ by version")
	}
}

func TestValidateConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysqld-auto.cnf")
	os.WriteFile(path, []byte("{\"mysql_server_uuid\":\"x\"}"), 0o600)
	if err := ValidateConfigFile(path); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("# comment\nport = 3307\n"), 0o600)
	if err := ValidateConfigFile(path); err == nil {
		t.Fatal("forbidden port key accepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureDataDirMyCNFOnce(t *testing.T) {
	p := testPaths(t)
	data := DataPath(p, "mysql", "8.4")
	if err := EnsureDataDir(p, "mysql", data); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "my.cnf")
	first, _ := os.ReadFile(path)
	if !strings.Contains(string(first), "# Generated by Lair") {
		t.Fatalf("my.cnf = %s", first)
	}
	os.WriteFile(path, []byte("# user edit\n"), 0o600)
	EnsureDataDir(p, "mysql", data)
	second, _ := os.ReadFile(path)
	if string(second) != "# user edit\n" {
		t.Fatal("my.cnf overwritten")
	}
}

func TestCreateDBCommand(t *testing.T) {
	p := testPaths(t)
	pg, _ := Build(p, "postgres", "18.1", 5432, "N1")
	cmd, err := CreateDBCommand(pg, "acme_test")
	if err != nil || !strings.Contains(strings.Join(cmd, " "), `CREATE DATABASE "acme_test"`) {
		t.Fatalf("pg createdb = %v, %v", cmd, err)
	}
	my, _ := Build(p, "mysql", "8.4", 3306, "N1")
	cmd, err = CreateDBCommand(my, "acme_test")
	if err != nil || !strings.Contains(strings.Join(cmd, " "), "CREATE DATABASE IF NOT EXISTS") {
		t.Fatalf("mysql createdb = %v, %v", cmd, err)
	}
	if _, err := CreateDBCommand(pg, "bad-name;DROP"); err == nil {
		t.Fatal("unsafe db name accepted")
	}
}

func TestPortCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot bind loopback")
	}
	port := ln.Addr().(*net.TCPAddr).Port
	defer ln.Close()

	OwnerFunc = func(int) string { return "nginx" }
	defer func() { OwnerFunc = defaultPortOwner }()

	c := CheckPort(port)
	if c == nil || c.Owner != "nginx" || c.Suggestion <= port {
		t.Fatalf("conflict = %+v", c)
	}
	if !strings.Contains(c.String(), "nginx") || !strings.Contains(c.String(), "try --port") {
		t.Fatalf("message = %s", c.String())
	}
}
