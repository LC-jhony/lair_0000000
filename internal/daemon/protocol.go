// Package daemon's protocol defines the action names and payloads used over
// the local API. Both the daemon (which registers handlers) and the CLI (which
// calls actions, locally or remotely) import this file so the wire format has
// a single source of truth.
package daemon

import (
	"github.com/lair-sh/lair/internal/engine"
	"github.com/lair-sh/lair/internal/installer"
	"github.com/lair-sh/lair/internal/project"
)

// Action names.
const (
	ActionStatus           = "status"
	ActionVersions         = "versions"
	ActionInstallPHP       = "install-php"
	ActionInstallNode      = "install-node"
	ActionInstallService   = "install-service"
	ActionJob              = "job"
	ActionJobs             = "jobs"
	ActionJobCancel        = "job-cancel"
	ActionJobRetry         = "job-retry"
	ActionPHPUninstall     = "php-uninstall"
	ActionNodeUpdate       = "node-update"
	ActionNodeUninstall    = "node-uninstall"
	ActionComposerUpdate   = "composer-update"
	ActionExtAdd           = "ext-add"
	ActionExtRequired      = "ext-required"
	ActionNpmUpdate        = "npm-update"
	ActionIsolated         = "isolated"
	ActionUnpin            = "unpin"
	ActionUnisolate        = "unisolate"
	ActionLogs             = "logs"
	ActionLogsList         = "logs-list"
	ActionPHPUse           = "php-use"
	ActionNodeDefault      = "node-default"
	ActionServiceUse       = "service-use"
	ActionServiceRestart   = "service-restart"
	ActionServiceStop      = "service-stop"
	ActionServiceUninstall = "service-uninstall"
	ActionCreateDB         = "create-db"
	ActionEnsureDatabase   = "ensure-database"
	ActionDBDefault        = "db-default"
	ActionPark             = "park"
	ActionLink             = "link"
	ActionUnlink           = "unlink"
	ActionSecure           = "secure"
	ActionWeb              = "web"
	ActionProject          = "project"
	ActionPin              = "pin"
	ActionIsolate          = "isolate"
	ActionUpdate           = "update"
	ActionBootstrap        = "bootstrap"
	ActionFirstRun         = "first-run"
	ActionEnsureShims      = "ensure-shims"
	ActionUninstall        = "uninstall"
	ActionRestart          = "restart"
	ActionShutdown         = "shutdown"
	ActionMail             = "mail"
	ActionDumps            = "dumps"
	ActionDebugger         = "debugger"
	ActionSite             = "site"
)

// VersionRequest asks for the versions available for a package.
type VersionRequest struct {
	Package string `json:"package"`
}

// VersionResult lists available versions, newest first.
type VersionResult struct {
	Versions []string `json:"versions"`
}

// WebRequest and WebResult are the wire payloads for the single web action
// shared by the desktop app, the TUI and the CLI. Supported ops are status,
// configure, https-on and https-off; the ports, bind and domain are configured
// through "configure" (or the CLI's http/https-port/bind/domain shorthands).
type WebRequest = engine.WebRequest
type WebResult = engine.WebResult

// ProjectRequest and ProjectResult are the wire payloads of the single project
// action: `info` reports capabilities and `create` starts a job that is polled
// with job/logs/job-cancel.
type ProjectRequest = project.ControlRequest

type ProjectResult = project.Result

// InstallRequest installs one tool or service version.
type InstallRequest struct {
	Version string `json:"version"`
	ID      string `json:"id,omitempty"`
	Port    int    `json:"port,omitempty"`
	// From selects a local directory or archive instead of a signed download.
	// Local builds are probed before being placed, like downloaded ones.
	From string `json:"from,omitempty"`
}

// JobRequest references a job id.
type JobRequest struct {
	ID string `json:"id"`
}

// LogsRequest names a log file.
type LogsRequest struct {
	Name string `json:"name"`
}

// SecureRequest toggles HTTPS for one site. On nil means "enable" for
// compatibility with older clients that only send a name.
type SecureRequest struct {
	Name string `json:"name"`
	On   *bool  `json:"on,omitempty"`
}

// LogsResult is the contents of a log file.
type LogsResult struct {
	Text string `json:"text"`
}

// NameList is a reusable list of names. Sources is populated by `logs-list`
// so a client can offer a tree of site and service logs; Items stays unchanged
// for older clients that only need the contents of LAIR_HOME/log.
type NameList struct {
	Items   []string    `json:"items"`
	Sources []LogSource `json:"sources,omitempty"`
}

// LogSource describes one group of discoverable logs (sites or services).
type LogSource = engine.LogSource

// Mail, dumps and debugger share their wire types with the engine, which owns
// the long-lived components. Aliases keep a single source of truth for the
// JSON contract used by the desktop app, the TUI and the CLI.
type MailRequest = engine.MailRequest
type MailResult = engine.MailResult
type MailMessage = engine.MailMessage
type MailAttachment = engine.MailAttachment

type DumpsRequest = engine.DumpsRequest
type DumpsResult = engine.DumpsResult
type DumpsOptions = engine.DumpsOptions
type DumpEvent = engine.DumpEvent

type DebuggerRequest = engine.DebuggerRequest
type DebuggerResult = engine.DebuggerResult
type DebuggerVersion = engine.DebuggerVersion

// PHPUseRequest sets the default PHP minor line.
type PHPUseRequest struct {
	Minor string `json:"minor"`
}

// NodeDefaultRequest sets the default Node version.
type NodeDefaultRequest struct {
	Version string `json:"version"`
}

// ServiceRequest targets one service id.
type ServiceRequest struct {
	ID string `json:"id"`
}

// ServiceUseRequest changes the active version of a service.
type ServiceUseRequest struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// ServiceUninstallRequest removes an installed service version.
type ServiceUninstallRequest struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	DeleteData bool   `json:"delete_data"`
}

// CreateDBRequest creates a database in a service.
type CreateDBRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DBDefaultRequest selects the database used by default in new projects.
type DBDefaultRequest struct {
	ID string `json:"id"`
}

// TextResult carries a command's textual output.
type TextResult struct {
	Text string `json:"text"`
}

// LinkRequest registers a site.
type LinkRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// UnlinkRequest removes a site from routing. DeleteFiles is opt-in: older
// clients only send `name` and keep their files untouched.
type UnlinkRequest struct {
	Name        string `json:"name"`
	DeleteFiles bool   `json:"delete_files,omitempty"`
}

// Site, mail, dumps and debugger share their wire types with the engine, which
// owns the long-lived state. Aliases keep a single source of truth for the JSON
// contract used by the desktop app, the TUI and the CLI.
type SiteRequest = engine.SiteRequest
type SiteResult = engine.SiteResult
type SiteDetail = engine.SiteDetail
type SiteEnvEntry = engine.SiteEnvEntry

// PathRequest carries a filesystem path.
type PathRequest struct {
	Path string `json:"path"`
}

// PinRequest writes a version pin.
type PinRequest struct {
	Tool  string `json:"tool"`
	Value string `json:"value"`
	Dir   string `json:"dir,omitempty"`
}

// IsolateRequest pins a version for a site.
type IsolateRequest struct {
	Tool  string `json:"tool"`
	Value string `json:"value"`
	Dir   string `json:"dir,omitempty"`
}

// ExtRequest names a PHP extension and the project that requires it.
type ExtRequest struct {
	Ext string `json:"ext"`
	Dir string `json:"dir,omitempty"`
}

// ExtensionInfo lists ext-* requirements and the ones missing from the PHP
// selected for a project.
type ExtensionInfo struct {
	Required []string `json:"required"`
	Missing  []string `json:"missing"`
}

// BootstrapRequest installs the local shims, profiles and Composer phar.
type BootstrapRequest struct {
	LairBinary string `json:"lair_binary"`
	Home       string `json:"home,omitempty"`
}

// EnsureShimsRequest regenerates the shims and PATH snippets. It is called by
// every interface after a runtime install, not only by the CLI, so `node -v`
// works regardless of which UI performed the installation.
type EnsureShimsRequest struct {
	LairBinary string `json:"lair_binary,omitempty"`
	Home       string `json:"home,omitempty"`
}

// BootstrapResult reports the paths created so the CLI can render the
// privileged plan. Warnings hold non-fatal problems (e.g. Composer offline).
type BootstrapResult struct {
	BinDir       string   `json:"bin_dir"`
	ComposerPath string   `json:"composer_path"`
	Warnings     []string `json:"warnings,omitempty"`
}

// FirstRunResult lists the jobs started by the fresh-install setup. Callers
// poll them to render a single progress bar for the whole operation.
type FirstRunResult struct {
	Jobs []installer.Status `json:"jobs"`
}

// UninstallRequest controls how Lair removes its local state.
type UninstallRequest struct {
	Home       string `json:"home,omitempty"`
	DataDir    string `json:"data_dir"`
	DeleteData bool   `json:"delete_data"`
}
