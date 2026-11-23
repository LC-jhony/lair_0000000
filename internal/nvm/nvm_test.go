package nvm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/app"
)

func TestExecEnvironmentRemovesLairShimsFromPath(t *testing.T) {
	paths, err := app.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", paths.Bin+string(os.PathListSeparator)+"/usr/bin")
	env := execEnvironment([]string{"exec", "v22.16.0", "npm", "-v"}, "/tmp/nvm")
	path := envValue(env, "PATH")
	if strings.Contains(path, paths.Bin) {
		t.Fatalf("PATH still contains the shim directory: %s", path)
	}
	if !strings.Contains(path, "/usr/bin") {
		t.Fatalf("PATH lost the system entries: %s", path)
	}
	if envValue(env, "NVM_DIR") != "/tmp/nvm" && !containsEnv(env, "NVM_DIR=/tmp/nvm") {
		t.Fatalf("NVM_DIR missing from %v", env)
	}
}

func containsEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}

func TestValidateArgs(t *testing.T) {
	if err := ValidateArgs([]string{"install", "lts/*"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateArgs([]string{"exec", "22.16.0", "npm", "-v"}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{},
		{"install", "22.16.0; rm -rf /"},
		{"install", "$(id)"},
		{"install", "`id`"},
		{"install", "22.16.0 && whoami"},
		{"install", "22.16.0\nwhoami"},
	} {
		if err := ValidateArgs(args); err == nil {
			t.Errorf("accepted unsafe arguments %q", args)
		}
	}
}

func TestExecSourcesNVM(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "nvm.sh")
	body := "nvm() { printf 'arg:%s\\n' \"$@\"; }\n"
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NVM_DIR", dir)

	out, err := Exec(context.Background(), "install", "22.16.0")
	if err != nil {
		t.Fatalf("Exec: %v (%s)", err, out)
	}
	for _, want := range []string{"arg:install", "arg:22.16.0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q does not contain %q", out, want)
		}
	}
}

func TestInstalledVersions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nvm.sh"), []byte("# nvm stub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NVM_DIR", dir)
	for _, ver := range []string{"v22.16.0", "v26.13.1", "not-a-version"} {
		bin := filepath.Join(dir, "versions", "node", ver, "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := InstalledVersions()
	if strings.Join(got, ",") != "v26.13.1,v22.16.0" {
		t.Fatalf("InstalledVersions = %v", got)
	}
	if path, ok := Installed("v22.16.0"); !ok || !strings.HasSuffix(path, "/v22.16.0") {
		t.Fatalf("Installed = %q, %v", path, ok)
	}
	if NewInstalled([]string{"v22.16.0"}, got) != "v26.13.1" {
		t.Fatal("NewInstalled did not find the new version")
	}
}

func TestRunBinaryGoesThroughLair(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "lair")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGS_FILE\"\n"
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARGS_FILE", argsFile)

	if _, err := RunBinary(context.Background(), bin, "install", "lts/*"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(strings.Fields(string(data)), ",") != "nvm,install,lts/*" {
		t.Fatalf("lair received args %q", data)
	}
}
