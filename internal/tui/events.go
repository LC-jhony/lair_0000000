package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/lair-sh/lair/internal/daemon"
)

type snapshotMsg struct{ snap Snapshot }
type tickMsg struct{ at time.Time }
type actionDoneMsg struct {
	label  string
	detail string
	err    error
}
type versionsMsg struct {
	versions map[string][]string
	err      error
}
type logTailMsg struct {
	name string
	text string
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg{at: t} })
}

func (m *Model) snapshotCmd() tea.Cmd {
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	caller := m.caller
	return func() tea.Msg {
		defer cancel()
		return snapshotMsg{snap: loadSnapshot(ctx, caller)}
	}
}

// actionCmd runs one daemon action outside the event loop. The summary is
// shown as a toast, the detail is kept for the status line.
func (m *Model) actionCmd(label string, fn func(context.Context, daemon.Caller) (string, error)) tea.Cmd {
	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
	caller := m.caller
	return func() tea.Msg {
		defer cancel()
		detail, err := fn(ctx, caller)
		if err != nil {
			return actionDoneMsg{label: label, detail: detail, err: err}
		}
		if detail == "" {
			detail = "ok"
		}
		return actionDoneMsg{label: label, detail: detail}
	}
}

func (m *Model) callCmd(label, action string, payload any) tea.Cmd {
	return m.actionCmd(label, func(ctx context.Context, c daemon.Caller) (string, error) {
		var out any
		if err := c.Call(ctx, action, payload, &out); err != nil {
			return "", err
		}
		return humanResult(out), nil
	})
}

// humanResult turns the small result types used by the dispatcher into a
// one-line summary. Unknown results simply report success.
func humanResult(v any) string {
	switch r := v.(type) {
	case nil:
		return ""
	case string:
		return r
	case daemon.TextResult:
		return r.Text
	case daemon.ProjectResult:
		if r.Error != "" {
			return r.Error
		}
		if r.ID != "" {
			return "job " + r.ID
		}
		return r.Output
	case interface{ String() string }:
		return r.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (m *Model) loadVersionsCmd(pkg string) tea.Cmd {
	ctx, cancel := context.WithTimeout(m.ctx, 25*time.Second)
	caller := m.caller
	return func() tea.Msg {
		defer cancel()
		var result daemon.VersionResult
		err := caller.Call(ctx, daemon.ActionVersions, daemon.VersionRequest{Package: pkg}, &result)
		return versionsMsg{versions: map[string][]string{pkg: result.Versions}, err: err}
	}
}
