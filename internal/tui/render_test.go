package tui

import (
	"context"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type emptyCaller struct{}

func (emptyCaller) Call(_ context.Context, _ string, _, out any) error {
	v := reflect.ValueOf(out)
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		v.Elem().Set(reflect.Zero(v.Elem().Type()))
	}
	return nil
}

func testModel(t *testing.T) *Model {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m := newModel(ctx, cancel, emptyCaller{})
	m.width, m.height = 120, 40
	m.snap = loadSnapshot(ctx, emptyCaller{})
	m.nav = m.buildNav()
	return m
}

func TestRenderAllPagesAtSeveralSizes(t *testing.T) {
	pages := []page{
		pageDashboard, pageRuntimes, pageDatabases, pageServices, pageSites,
		pageMail, pageDumps, pageDebugger, pageLogs, pageSystem,
	}
	sizes := [][2]int{{60, 14}, {100, 30}, {160, 50}}
	for _, p := range pages {
		for _, size := range sizes {
			m := testModel(t)
			m.page = p
			m.width, m.height = size[0], size[1]
			view := m.View()
			if view.Content == "" {
				t.Fatalf("empty view for page %v at %v", p, size)
			}
			if !view.AltScreen || view.MouseMode != tea.MouseModeCellMotion {
				t.Fatalf("view flags missing for page %v: %+v", p, view)
			}
		}
	}
}

func TestRenderTooSmall(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 40, 8
	if v := m.View(); v.Content == "" {
		t.Fatal("expected resize hint")
	}
}

func TestFuzzyFilter(t *testing.T) {
	cases := []struct {
		filter, label string
		want          bool
	}{
		{"", "anything", true},
		{"inst php", "Install PHP version", true},
		{"php inst", "Install PHP version", false},
		{"site", "Site: blog.test", true},
		{"xyz", "Install PHP version", false},
	}
	for _, tc := range cases {
		if got := fuzzy(tc.filter, tc.label); got != tc.want {
			t.Errorf("fuzzy(%q,%q) = %v, want %v", tc.filter, tc.label, got, tc.want)
		}
	}
}

func TestPaletteNeverOffersDestructiveActions(t *testing.T) {
	m := testModel(t)
	for _, e := range m.paletteEntries() {
		for _, forbidden := range []string{"uninstall", "remove site", "delete", "wipe"} {
			if containsLower(e.label, forbidden) {
				t.Errorf("destructive palette entry: %q", e.label)
			}
		}
	}
}

func TestKeyNavigationChangesFocus(t *testing.T) {
	m := testModel(t)
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focus != focusMain {
		t.Fatal("tab did not move focus to main")
	}
	m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	if !m.helpOn {
		t.Fatal("? did not open help")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.helpOn {
		t.Fatal("esc did not close help")
	}
}

func containsLower(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexLower(s, sub) >= 0
}

func indexLower(s, sub string) int {
	runes := []rune(s)
	needle := []rune(sub)
	for i := 0; i+len(needle) <= len(runes); i++ {
		ok := true
		for j := range needle {
			if lowerRune(runes[i+j]) != lowerRune(needle[j]) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func lowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 32
	}
	return r
}
