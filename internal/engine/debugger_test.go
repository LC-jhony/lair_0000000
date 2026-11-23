package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lair-sh/lair/internal/config"
)

func TestDebuggerStatusAndToggles(t *testing.T) {
	e, srv := testEnv(t)
	defer srv.Close()
	ctx := context.Background()

	if err := e.Config.Update(func(c *config.Config) error {
		c.Sites["acme.test"] = config.Site{Path: "/srv/acme"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for minor, withXdebug := range map[string]bool{"8.4": true, "8.3": false} {
		dir := filepath.Join(e.Paths.PHP, minor)
		if err := os.MkdirAll(filepath.Join(dir, "lib", "php", "extensions"), 0o755); err != nil {
			t.Fatal(err)
		}
		marker := `{"version":"` + minor + `.3","build":"N1","minor":"` + minor + `"}`
		if err := os.WriteFile(filepath.Join(dir, "lair.json"), []byte(marker), 0o644); err != nil {
			t.Fatal(err)
		}
		if withXdebug {
			if err := os.WriteFile(filepath.Join(dir, "lib", "php", "extensions", "xdebug.so"), []byte("module"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	st, err := e.DebuggerControl(ctx, DebuggerRequest{Op: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Port != 9003 || st.Mode != "debug" || st.Detect || st.Breakpoints {
		t.Fatalf("status = %+v", st)
	}
	if len(st.Domains) != 1 || st.Domains[0] != "acme.test" {
		t.Fatalf("domains = %v", st.Domains)
	}
	available := map[string]bool{}
	for _, v := range st.Versions {
		available[v.Version] = v.Available
	}
	if !available["8.4"] || available["8.3"] {
		t.Fatalf("versions = %+v", st.Versions)
	}

	for _, op := range []string{"detect-on", "breakpoints-on"} {
		if _, err := e.DebuggerControl(ctx, DebuggerRequest{Op: op}); err != nil {
			t.Fatal(err)
		}
	}
	c, err := e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Debugger.Detect || !c.Debugger.Breakpoints {
		t.Fatalf("persisted = %+v", c.Debugger)
	}
	st, err = e.DebuggerControl(ctx, DebuggerRequest{Op: "detect-off"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Detect || !st.Breakpoints {
		t.Fatalf("after detect-off = %+v", st)
	}
	if _, err := e.DebuggerControl(ctx, DebuggerRequest{Op: "breakpoints-off"}); err != nil {
		t.Fatal(err)
	}
	c, err = e.Config.View()
	if err != nil {
		t.Fatal(err)
	}
	if c.Debugger.Detect || c.Debugger.Breakpoints {
		t.Fatalf("after both off = %+v", c.Debugger)
	}
	if _, err := e.DebuggerControl(ctx, DebuggerRequest{Op: "nope"}); err == nil {
		t.Fatal("unknown op accepted")
	}
}
