package services

import (
	"strings"
	"testing"
)

func TestSpecCredentialDefaults(t *testing.T) {
	// t.Setenv("") exercises the fallback even if the CI runner sets a value.
	t.Setenv(typesenseAPIKeyEnv, "")
	t.Setenv(rustFSUserEnv, "")
	t.Setenv(rustFSPasswordEnv, "")
	p := testPaths(t)

	ts, err := Build(p, "typesense", "8.4", DefaultPort("typesense"), "N1")
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(ts.Args, "--api-key="+defaultTypesenseAPIKey) ||
		!strings.Contains(ts.EnvSnippet, "TYPESENSE_API_KEY="+defaultTypesenseAPIKey) {
		t.Fatalf("typesense defaults missing: args=%v snippet=%q", ts.Args, ts.EnvSnippet)
	}

	fs, err := Build(p, "rustfs", "8.4", DefaultPort("rustfs"), "N1")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(fs.Args, " ")
	if !strings.Contains(args, "--access-key "+defaultRustFSUser) ||
		!strings.Contains(args, "--secret-key "+defaultRustFSPassword) ||
		!strings.Contains(fs.EnvSnippet, "AWS_ACCESS_KEY_ID="+defaultRustFSUser) ||
		!strings.Contains(fs.EnvSnippet, "AWS_SECRET_ACCESS_KEY="+defaultRustFSPassword) {
		t.Fatalf("rustfs defaults missing: args=%v snippet=%q", fs.Args, fs.EnvSnippet)
	}
}

func TestSpecCredentialOverrides(t *testing.T) {
	t.Setenv(typesenseAPIKeyEnv, "secret-ts")
	t.Setenv(rustFSUserEnv, "custom-user")
	t.Setenv(rustFSPasswordEnv, "custom-pass")
	p := testPaths(t)

	ts, err := Build(p, "typesense", "8.4", DefaultPort("typesense"), "N1")
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(ts.Args, "--api-key=secret-ts") ||
		!strings.Contains(ts.EnvSnippet, "TYPESENSE_API_KEY=secret-ts") ||
		strings.Contains(strings.Join(ts.Args, " "), "--api-key=lair") ||
		strings.Contains(ts.EnvSnippet, "TYPESENSE_API_KEY=lair") {
		t.Fatalf("typesense override not applied: args=%v snippet=%q", ts.Args, ts.EnvSnippet)
	}

	fs, err := Build(p, "rustfs", "8.4", DefaultPort("rustfs"), "N1")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(fs.Args, " ")
	if !strings.Contains(args, "--access-key custom-user") ||
		!strings.Contains(args, "--secret-key custom-pass") ||
		!strings.Contains(fs.EnvSnippet, "AWS_ACCESS_KEY_ID=custom-user") ||
		!strings.Contains(fs.EnvSnippet, "AWS_SECRET_ACCESS_KEY=custom-pass") ||
		strings.Contains(args, "--access-key rustfsadmin") ||
		strings.Contains(args, "--secret-key rustfsadmin") ||
		strings.Contains(fs.EnvSnippet, "AWS_ACCESS_KEY_ID=rustfsadmin") ||
		strings.Contains(fs.EnvSnippet, "AWS_SECRET_ACCESS_KEY=rustfsadmin") {
		t.Fatalf("rustfs override not applied: args=%v snippet=%q", fs.Args, fs.EnvSnippet)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
