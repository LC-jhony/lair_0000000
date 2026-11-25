package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func stackSite(t *testing.T, name string) (*Engine, string) {
	t.Helper()
	e, _ := testEnv(t)
	return e, linkTestSite(t, e, name)
}

func TestStackControlWordPressSQLite(t *testing.T) {
	e, dir := stackSite(t, "wp-stack.test")
	writeTestFile(t, filepath.Join(dir, "wp-config.php"), `
define( 'DB_NAME', 'wp_dev' );
define( 'DB_HOST', '127.0.0.1' );
$table_prefix = 'wp_';
define( 'DB_PASSWORD', 'super-secreto' );
`)
	writeTestFile(t, filepath.Join(dir, "index.php"), "<?php")
	writeTestFile(t, filepath.Join(dir, "wp-content", "db.php"), "<?php // drop-in")
	writeTestFile(t, filepath.Join(dir, "wp-includes", "version.php"), "<?php $wp_version = '6.8.2';")

	s := e.StackControl(context.Background(), "wp-stack.test")
	if s.Framework != "wordpress" || s.FrameworkVersion != "6.8.2" {
		t.Fatalf("framework = %+v", s)
	}
	if !strings.Contains(s.Database, "sqlite") || s.DatabaseName != "wp_dev" || s.DatabaseHost != "127.0.0.1" {
		t.Fatalf("database = %+v", s)
	}
	if s.WebRoot != dir || s.FrontController != "index.php" {
		t.Fatalf("web root = %q front = %q", s.WebRoot, s.FrontController)
	}
	encoded, _ := json.Marshal(s)
	if strings.Contains(string(encoded), "super-secreto") {
		t.Fatal("password leaked into stack info")
	}
}

func TestStackControlJoomlaPDO(t *testing.T) {
	e, dir := stackSite(t, "joomla-stack.test")
	writeTestFile(t, filepath.Join(dir, "index.php"), "<?php")
	writeTestFile(t, filepath.Join(dir, "configuration.php"), `<?php
class JConfig {
  public $dbtype = 'mysql';
  public $db = 'joomla_dev';
  public $host = '127.0.0.1';
  public $dbprefix = 'joomla_';
  public $password = 'super-secreto';
}`)
	writeTestFile(t, filepath.Join(dir, "libraries", "src", "Version.php"), `<?php
class Version {
  public const MAJOR_VERSION = 6;
  public const MINOR_VERSION = 2;
  public const PATCH_VERSION = 1;
}`)

	s := e.StackControl(context.Background(), "joomla-stack.test")
	if s.Framework != "joomla" || s.FrameworkVersion != "6.2.1" {
		t.Fatalf("framework = %+v", s)
	}
	if !strings.Contains(s.Database, "PDO") || s.DatabaseName != "joomla_dev" || s.DatabaseHost != "127.0.0.1" {
		t.Fatalf("database = %+v", s)
	}
	encoded, _ := json.Marshal(s)
	if strings.Contains(string(encoded), "super-secreto") {
		t.Fatal("password leaked into stack info")
	}
}

func TestStackControlSymfony(t *testing.T) {
	e, dir := stackSite(t, "symfony-stack.test")
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"symfony/framework-bundle":"^7.2"}}`)
	writeTestFile(t, filepath.Join(dir, "public", "index.php"), "<?php")
	writeTestFile(t, filepath.Join(dir, ".env"), "DB_PASSWORD=super-secreto\nDB_CONNECTION=pgsql\nDB_DATABASE=app\n")

	s := e.StackControl(context.Background(), "symfony-stack.test")
	if s.Framework != "symfony" || s.FrameworkVersion != "7.2" || s.Database != "pgsql" || s.DatabaseName != "app" {
		t.Fatalf("stack = %+v", s)
	}
	if s.WebRoot != filepath.Join(dir, "public") || s.FrontController != "index.php" {
		t.Fatalf("web root = %q front = %q", s.WebRoot, s.FrontController)
	}
	encoded, _ := json.Marshal(s)
	if strings.Contains(string(encoded), "super-secreto") {
		t.Fatal("password leaked into stack info")
	}
}
