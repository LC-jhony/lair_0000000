package tui

import (
	"context"
	"time"

	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/project"
)

// Snapshot is an immutable projection of the daemon's state. It is rebuilt
// from scratch on every refresh; views never mutate it and never keep their
// own copy of domain data.
type Snapshot struct {
	Status      engine.Status
	Health      []engine.ServiceHealth
	Jobs        []installer.Status
	LogNames    []string
	LogSources  []engine.LogSource
	SiteData    map[string]engine.SiteData
	Stacks      map[string]engine.StackInfo
	Queues      map[string]engine.QueueInfo
	Details     map[string]engine.SiteDetail
	Project     project.Info
	ProjectDirs []string
	Mail        engine.MailResult
	Dumps       engine.DumpsResult
	Debugger    engine.DebuggerResult
	Warnings    []string
	Updated     time.Time
}

// loadSnapshot gathers all data used by the dashboard. Every field comes from
// a daemon action; failures degrade to an empty value plus a warning so a
// single unsupported action cannot blank the screen.
func loadSnapshot(ctx context.Context, caller daemon.Caller) Snapshot {
	s := Snapshot{
		SiteData: map[string]engine.SiteData{},
		Stacks:   map[string]engine.StackInfo{},
		Queues:   map[string]engine.QueueInfo{},
		Details:  map[string]engine.SiteDetail{},
		Warnings: []string{},
		Updated:  time.Now(),
	}
	call := func(action string, payload, out any) bool {
		if err := caller.Call(ctx, action, payload, out); err != nil {
			s.Warnings = append(s.Warnings, action+": "+err.Error())
			return false
		}
		return true
	}

	call(daemon.ActionStatus, nil, &s.Status)
	call(daemon.ActionJobs, nil, &s.Jobs)
	call(daemon.ActionServicesHealth, nil, &s.Health)
	var logs daemon.NameList
	if call(daemon.ActionLogsList, nil, &logs) {
		s.LogNames, s.LogSources = logs.Items, logs.Sources
	}
	var projectResult daemon.ProjectResult
	if call(daemon.ActionProject, daemon.ProjectRequest{Op: "info"}, &projectResult) {
		s.Project = projectResult.Info
	}
	if call(daemon.ActionProject, daemon.ProjectRequest{Op: "dirs-list"}, &projectResult) {
		s.ProjectDirs = projectResult.Dirs
	}
	// "list" also carries Running/Capturing/Count/Options, and is the only
	// op that fills Messages/Events. "status" would render empty debug panes.
	call(daemon.ActionMail, daemon.MailRequest{Op: "list"}, &s.Mail)
	call(daemon.ActionDumps, daemon.DumpsRequest{Op: "list"}, &s.Dumps)
	call(daemon.ActionDebugger, daemon.DebuggerRequest{Op: "status"}, &s.Debugger)

	for _, site := range s.Status.Sites {
		name := site.Name
		var detail daemon.SiteResult
		if call(daemon.ActionSite, daemon.SiteRequest{Op: "detail", Name: name}, &detail) && detail.Detail != nil {
			s.Details[name] = *detail.Detail
		}
		var data engine.SiteData
		if call(daemon.ActionSiteData, daemon.SiteDataRequest{Name: name}, &data) {
			s.SiteData[name] = data
		}
		var queue engine.QueueInfo
		if call(daemon.ActionQueue, daemon.QueueRequest{Name: name}, &queue) {
			s.Queues[name] = queue
		}
		var stack engine.StackInfo
		if call(daemon.ActionSiteStack, daemon.SiteStackRequest{Name: name}, &stack) {
			s.Stacks[name] = stack
		}
	}
	return s
}

// attention returns the number of unhealthy items in the snapshot.
func (s Snapshot) attention() int {
	n := 0
	if s.Status.Web.Error != "" || s.Status.Web.Warning != "" {
		n++
	}
	if s.Status.DNS.Host != "" && !s.Status.DNS.OK {
		n++
	}
	for _, h := range s.Health {
		if !h.Healthy {
			n++
		}
	}
	for _, j := range s.Jobs {
		if j.Error != "" && !j.Canceled {
			n++
		}
	}
	for _, d := range s.Details {
		if d.RootState != "" && d.RootState != "ok" {
			n++
		}
	}
	return n
}
