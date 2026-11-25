package php

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectConstraintPrefersPHPVersionFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".php-version"), []byte("8.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"php":"^8.1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := DetectConstraint(dir)
	if err != nil || got != "8.3" {
		t.Fatalf("constraint = %q, %v; want 8.3", got, err)
	}
}

func TestDetectChoosesNewestSatisfyingMinor(t *testing.T) {
	cases := []struct {
		name       string
		constraint string
		installed  []string
		want       string
	}{
		{"caret", "^8.3", []string{"8.2", "8.3", "8.4"}, "8.4"},
		{"min", ">=8.2", []string{"8.1", "8.4"}, "8.4"},
		{"wildcard", "8.3.*", []string{"8.2", "8.3", "8.4"}, "8.3"},
		{"alternatives", "^7.4 || ^8.2", []string{"8.1", "8.2", "8.4"}, "8.4"},
		{"none", "", []string{"8.4"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.constraint != "" {
				doc := `{"require":{"php":"` + tc.constraint + `"}}`
				if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(doc), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			installed := make([]Info, 0, len(tc.installed))
			for _, minor := range tc.installed {
				installed = append(installed, Info{Minor: minor, Dir: filepath.Join(dir, minor)})
			}
			got, _, err := Detect(dir, installed)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("Detect() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectFailsWhenNothingSatisfies(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"php":"^8.4"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Detect(dir, []Info{{Minor: "8.2"}}); err == nil {
		t.Fatal("expected an error when no installed version matches")
	}
}
