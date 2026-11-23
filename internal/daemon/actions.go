package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/installer"
)

// Caller is implemented by both the remote API Client and the in-process
// Dispatcher, letting the CLI talk to a daemon or run standalone.
type Caller interface {
	Call(ctx context.Context, action string, payload, out any) error
}

// Dispatcher dispatches actions directly to an engine. It is the in-process
// counterpart of the unix-socket API, used by the CLI when no daemon runs.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string]Action
}

// NewDispatcher registers every action against an engine.
func NewDispatcher(e *engine.Engine, shutdown func()) *Dispatcher {
	d := &Dispatcher{handlers: map[string]Action{}}

	add := func(name string, fn func(context.Context, json.RawMessage) (any, error)) {
		d.handlers[name] = fn
	}

	add(ActionStatus, func(context.Context, json.RawMessage) (any, error) { return e.Status() })
	add(ActionVersions, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req VersionRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		versions, err := e.Available(req.Package)
		if err != nil {
			return nil, err
		}
		return VersionResult{Versions: versions}, nil
	})
	add(ActionInstallPHP, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req InstallRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		var job *installer.Job
		var err error
		if req.From != "" {
			job, err = e.InstallPHPFrom(jobContext(ctx), req.Version, req.From)
		} else {
			job, err = e.InstallPHP(jobContext(ctx), req.Version)
		}
		if err != nil {
			return nil, err
		}
		return job.Status(), nil
	})
	add(ActionInstallNode, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req InstallRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		var job *installer.Job
		var err error
		if req.From != "" {
			job, err = e.InstallNodeFrom(jobContext(ctx), req.From)
		} else {
			job, err = e.InstallNode(jobContext(ctx), req.Version)
		}
		if err != nil {
			return nil, err
		}
		return job.Status(), nil
	})
	add(ActionInstallService, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req InstallRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		job, err := e.InstallService(jobContext(ctx), req.ID, req.Version, req.Port)
		if err != nil {
			return nil, err
		}
		return job.Status(), nil
	})
	add(ActionJob, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req JobRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		status, ok := e.Installer.JobStatus(req.ID)
		if !ok {
			return nil, fmt.Errorf("no job %q", req.ID)
		}
		return status, nil
	})
	add(ActionJobs, func(context.Context, json.RawMessage) (any, error) { return e.Installer.List(), nil })
	add(ActionJobCancel, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req JobRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return map[string]bool{"canceled": e.CancelJob(req.ID)}, nil
	})
	add(ActionJobRetry, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req JobRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		// Like other job actions, a retry must outlive the HTTP request.
		job, err := e.Installer.Retry(jobContext(ctx), req.ID)
		if err != nil {
			return nil, err
		}
		return job.Status(), nil
	})
	add(ActionLogs, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req LogsRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		text, err := e.Logs(req.Name)
		if err != nil {
			return nil, err
		}
		return LogsResult{Text: text}, nil
	})
	add(ActionLogsList, func(context.Context, json.RawMessage) (any, error) {
		items, err := e.ListLogs()
		if err != nil {
			return nil, err
		}
		sources, err := e.ListLogSources()
		if err != nil {
			return nil, err
		}
		return NameList{Items: items, Sources: sources}, nil
	})
	add(ActionMail, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req MailRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return e.MailControl(ctx, req)
	})
	add(ActionDumps, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req DumpsRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return e.DumpsControl(ctx, req)
	})
	add(ActionDebugger, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req DebuggerRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return e.DebuggerControl(ctx, req)
	})
	add(ActionPHPUse, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req PHPUseRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.PHPUse(req.Minor)
	})
	add(ActionNodeDefault, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req NodeDefaultRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.NodeDefault(req.Version)
	})
	add(ActionPHPUninstall, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req PHPUseRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.PHPUninstall(req.Minor)
	})
	add(ActionNodeUpdate, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req InstallRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		job, err := e.NodeUpdate(jobContext(ctx), req.Version)
		if err != nil {
			return nil, err
		}
		return job.Status(), nil
	})
	add(ActionNodeUninstall, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req NodeDefaultRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.NodeUninstall(req.Version)
	})
	add(ActionComposerUpdate, func(ctx context.Context, _ json.RawMessage) (any, error) {
		path, err := e.ComposerUpdate(ctx)
		if err != nil {
			return nil, err
		}
		return TextResult{Text: path}, nil
	})
	add(ActionExtAdd, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req ExtRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		out, err := e.ExtAdd(ctx, req.Dir, req.Ext)
		if err != nil {
			return TextResult{Text: out}, fmt.Errorf("%w: %s", err, out)
		}
		return TextResult{Text: out}, nil
	})
	add(ActionExtRequired, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req PathRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		if req.Path == "" {
			req.Path, _ = os.Getwd()
		}
		required, missing, err := e.ExtRequired(req.Path)
		if err != nil {
			return nil, err
		}
		return ExtensionInfo{Required: required, Missing: missing}, nil
	})
	add(ActionNpmUpdate, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req PathRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		if req.Path == "" {
			req.Path, _ = os.Getwd()
		}
		ver, err := e.NpmUpdate(ctx, req.Path)
		if err != nil {
			return nil, err
		}
		return TextResult{Text: ver}, nil
	})
	add(ActionIsolated, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req PathRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		if req.Path == "" {
			req.Path, _ = os.Getwd()
		}
		return map[string]string{
			"php":  e.IsolatedVersion("php", req.Path),
			"node": e.IsolatedVersion("node", req.Path),
		}, nil
	})
	add(ActionUnpin, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req PinRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.Unpin(req.Tool, req.Dir)
	})
	add(ActionUnisolate, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req IsolateRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.Isolate(req.Tool, "", req.Dir)
	})
	add(ActionServiceUse, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req ServiceUseRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.UseService(req.ID, req.Version)
	})
	add(ActionServiceRestart, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req ServiceRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.RestartService(req.ID)
	})
	add(ActionServiceStop, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req ServiceRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.StopService(req.ID)
	})
	add(ActionServiceUninstall, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req ServiceUninstallRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.UninstallService(req.ID, req.Version, req.DeleteData)
	})
	add(ActionCreateDB, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req CreateDBRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		out, err := e.CreateDatabaseContext(ctx, req.ID, req.Name)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", err, out)
		}
		return TextResult{Text: out}, nil
	})
	add(ActionEnsureDatabase, func(ctx context.Context, _ json.RawMessage) (any, error) {
		job, err := e.EnsureDefaultDatabase(jobContext(ctx))
		if err != nil {
			return nil, err
		}
		if job == nil {
			return map[string]bool{"already_installed": true}, nil
		}
		return job.Status(), nil
	})
	add(ActionDBDefault, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req DBDefaultRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.SetDefaultDatabase(req.ID)
	})
	add(ActionPark, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req PathRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		name, err := e.Park(req.Path)
		if err != nil {
			return nil, err
		}
		return TextResult{Text: name}, nil
	})
	add(ActionLink, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req LinkRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.Link(req.Name, req.Path)
	})
	add(ActionUnlink, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req UnlinkRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		if req.DeleteFiles {
			_, err := e.SiteControl(ctx, SiteRequest{Op: "delete", Name: req.Name, DeleteFiles: true})
			return nil, err
		}
		// Older clients send only `name`; unlink keeps project files intact.
		return nil, e.Unlink(req.Name)
	})
	add(ActionSite, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req SiteRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return e.SiteControl(ctx, req)
	})
	add(ActionSecure, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req SecureRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		on := req.On == nil || *req.On
		return nil, e.SetSiteHTTPS(req.Name, on)
	})
	add(ActionWeb, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req WebRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return e.WebControl(ctx, req)
	})
	add(ActionProject, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req ProjectRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return e.ProjectControlWith(jobContext(ctx), req)
	})
	add(ActionPin, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req PinRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.Pin(req.Tool, req.Value, req.Dir)
	})
	add(ActionIsolate, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req IsolateRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		return nil, e.Isolate(req.Tool, req.Value, req.Dir)
	})
	add(ActionUpdate, func(ctx context.Context, _ json.RawMessage) (any, error) {
		text, err := e.RefreshCatalog(ctx)
		if err != nil {
			return nil, err
		}
		return TextResult{Text: text}, nil
	})
	add(ActionBootstrap, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req BootstrapRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		// A client must never decide which binary the shims execute.
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		phar, warnings, err := e.Bootstrap(ctx, exe, req.Home)
		if err != nil {
			return nil, err
		}
		return BootstrapResult{BinDir: e.Paths.Bin, ComposerPath: phar, Warnings: warnings}, nil
	})
	add(ActionFirstRun, func(ctx context.Context, _ json.RawMessage) (any, error) {
		// Jobs must outlive the HTTP request that started them.
		jobs, err := e.FirstRun(jobContext(ctx))
		if err != nil {
			return nil, err
		}
		result := FirstRunResult{Jobs: make([]installer.Status, 0, len(jobs))}
		for _, job := range jobs {
			result.Jobs = append(result.Jobs, job.Status())
		}
		return result, nil
	})
	add(ActionEnsureShims, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req EnsureShimsRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		// Ignore lair_binary from the wire: only the running daemon's own
		// executable may be embedded in generated shims.
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		bin, err := e.EnsureShims(exe, req.Home)
		if err != nil {
			return nil, err
		}
		return map[string]string{"bin_dir": bin}, nil
	})
	add(ActionUninstall, func(_ context.Context, raw json.RawMessage) (any, error) {
		var req UninstallRequest
		if err := decode(raw, &req); err != nil {
			return nil, err
		}
		// home/data_dir in the payload are deliberately ignored by the engine.
		return nil, e.Uninstall("", "", req.DeleteData)
	})
	add(ActionRestart, func(context.Context, json.RawMessage) (any, error) {
		return map[string]bool{"restarting": true}, nil
	})
	add(ActionShutdown, func(context.Context, json.RawMessage) (any, error) {
		if shutdown != nil {
			go shutdown()
		}
		return map[string]bool{"shutting_down": true}, nil
	})
	return d
}

// Set overrides a handler. The runtime uses it to provide actions that need
// access to long-lived components (e.g. restarting all supervisors).
func (d *Dispatcher) Set(name string, fn Action) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[name] = fn
}

// Register installs every action on a Server.
func Register(srv *Server, d *Dispatcher) {
	d.mu.RLock()
	handlers := make(map[string]Action, len(d.handlers))
	for name, fn := range d.handlers {
		handlers[name] = fn
	}
	d.mu.RUnlock()
	for name, fn := range handlers {
		srv.Handle(name, fn)
	}
}

// Call implements Caller for in-process use.
func (d *Dispatcher) Call(ctx context.Context, action string, payload, out any) error {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		raw = b
	}
	d.mu.RLock()
	fn, ok := d.handlers[action]
	d.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown action: %s", action)
	}
	data, err := fn(ctx, raw)
	if err != nil {
		return err
	}
	if out == nil || data == nil {
		return nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// jobContext detaches a background job from the lifecycle of a single API
// request. The desktop bridge and the CLI both call through this dispatcher;
// neither may kill a running install merely because the HTTP reply was sent.
// Cancellation is still available through the explicit job-cancel action.
func jobContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

// decode fills out from an action payload. An omitted payload is valid (many
// actions take no parameters); malformed JSON is an error and must be surfaced
// by every handler instead of silently operating on a zero-value request.
func decode(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	return nil
}
