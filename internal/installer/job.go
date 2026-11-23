// Package installer coordinates verified downloads into installed versions.
// A job moves through "downloading" and "installing", exposes progress and
// cancellation, logs to log/<id>-install.log, and only touches config.json on
// success. At most one job runs per service id.
package installer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/download"
	"github.com/lair-sh/lair/internal/manifest"
)

// Request describes one installation.
type Request struct {
	ID        string // unique id, e.g. "php/8.4" or "postgres/18"
	Package   string // manifest package key
	Version   string
	Platform  string
	Artifact  manifest.Artifact
	Dest      string
	Expected  []string // executables that must exist after extraction
	ConfigKey string   // config.Installed key; empty skips config update

	// Preserve lists user-owned paths (relative to Dest) carried over from an
	// older install into staging before the atomic swap. Used for php.ini and
	// the conf.d directories, which upgrades must never destroy.
	Preserve []string
	// VerifyStaging checks the staged binaries before the swap. A wrong PHP
	// version (or a missing php-fpm) leaves the current install untouched.
	VerifyStaging func(staging string) error
	// MergeStaging reorganises a merged package (Windows PHP + extension pack)
	// inside staging before verification.
	MergeStaging func(staging string, archives []string) error

	// OnSuccess, when set, runs after the atomic swap, build marker and config
	// update. It is package-specific setup (markers, php.ini, data init). If it
	// fails the job is marked failed, but the installed files remain.
	OnSuccess func(dest string) error
}

// Status is a snapshot of a job.
type Status struct {
	ID         string `json:"id"`
	Package    string `json:"package,omitempty"`
	Version    string `json:"version,omitempty"`
	Phase      string `json:"phase"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Error      string `json:"error,omitempty"`
	Done       bool   `json:"done"`
	Canceled   bool   `json:"canceled"`
	Retryable  bool   `json:"retryable,omitempty"`
}

// Job is a cancellable install operation.
type Job struct {
	mu      sync.Mutex
	status  Status
	cancel  context.CancelFunc
	logFile *os.File
	req     Request
}

// Status returns a snapshot.
func (j *Job) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

func (j *Job) set(fn func(*Status)) {
	j.mu.Lock()
	fn(&j.status)
	j.mu.Unlock()
}

func (j *Job) log(line string) {
	j.mu.Lock()
	file := j.logFile
	j.mu.Unlock()
	if file != nil {
		fmt.Fprintln(file, line)
	}
}

// Cancel requests cancellation. The filesystem work stops at the next check.
func (j *Job) Cancel() {
	j.mu.Lock()
	cancel := j.cancel
	j.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Manager enforces one concurrent job per id.
type Manager struct {
	logDir string
	root   string // data root; installs must stay inside it
	dl     *download.Downloader
	cfg    *config.Store

	mu      sync.Mutex
	running map[string]*Job
	// onDone runs after every successful job. The engine uses it to refresh
	// shims/PATH so a newly installed tool is immediately reachable.
	onDone func()
	// lastReq keeps the request of a failed job so `job-retry` can start it
	// again without the caller having to rebuild the manifest artifact.
	lastReq map[string]Request
	// recent keeps the final status of finished jobs. The CLI polls ActionJob
	// after starting an install; without this, a fast install could finish and
	// be forgotten before the first poll, producing a spurious "no job" error.
	recent      map[string]Status
	recentOrder []string
}

const maxRecentJobs = 100

func NewManager(logDir string, dl *download.Downloader, cfg *config.Store) *Manager {
	return &Manager{
		logDir: logDir, root: filepath.Dir(logDir), dl: dl, cfg: cfg,
		running: map[string]*Job{}, recent: map[string]Status{},
		lastReq: map[string]Request{},
	}
}

// logFileName keeps install logs inside logDir and avoids collisions between
// ids that only differ in characters replaced during sanitising ("a/b" vs
// "a-b"). A short digest is appended only when sanitising actually happened.
func logFileName(id string) string {
	safe := strings.ReplaceAll(id, string(os.PathSeparator), "-")
	if safe != id {
		sum := sha256.Sum256([]byte(id))
		safe += "-" + hex.EncodeToString(sum[:])[:8]
	}
	return safe + "-install.log"
}

// ConfigStore returns the store shared with the engine. It is exposed so callers
// can verify that configuration writes are serialised by a single mutex.
func (m *Manager) ConfigStore() *config.Store { return m.cfg }

// Start begins an install. It returns an error when a job with the same id is
// already running, when the artifact is yanked, or when it has no sha256.
func (m *Manager) Start(parent context.Context, req Request) (*Job, error) {
	if err := manifest.VerifyArtifact(req.Artifact); err != nil {
		return nil, err
	}
	if req.ID == "" || req.Dest == "" {
		return nil, fmt.Errorf("job id and destination are required")
	}
	// Nothing Lair installs may escape the user's data root, even when a
	// caller supplies a destination from a config file or an API payload.
	rel, relErr := filepath.Rel(m.root, req.Dest)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("install destination %s is outside %s", req.Dest, m.root)
	}
	m.mu.Lock()
	if _, exists := m.running[req.ID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("an install for %s is already running", req.ID)
	}
	ctx, cancel := context.WithCancel(parent)
	job := &Job{status: Status{
		ID: req.ID, Package: req.Package, Version: req.Version, Phase: "downloading",
	}, req: req}
	job.cancel = cancel
	m.running[req.ID] = job
	m.mu.Unlock()

	if err := os.MkdirAll(m.logDir, 0o755); err != nil {
		m.finish(req.ID, job, err, false)
		return nil, err
	}
	logPath := filepath.Join(m.logDir, logFileName(req.ID))
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		m.finish(req.ID, job, err, false)
		return nil, err
	}
	job.mu.Lock()
	job.logFile = lf
	job.mu.Unlock()

	go m.run(ctx, req, job, cancel, logPath)
	return job, nil
}

func (m *Manager) run(ctx context.Context, req Request, job *Job, cancel context.CancelFunc, logPath string) {
	defer cancel()
	defer func() {
		job.mu.Lock()
		if job.logFile != nil {
			job.logFile.Close()
		}
		job.mu.Unlock()
	}()

	job.log(fmt.Sprintf("install %s %s (%s) build=%s", req.Package, req.Version, req.Platform, req.Artifact.Build))
	cacheDir := filepath.Dir(req.Dest)
	safeName := strings.ReplaceAll(req.ID, string(os.PathSeparator), "-") + "-" + req.Artifact.Build

	type archiveSource struct {
		url, sha, name string
		size           int64
		files          []manifest.FileMap
		raw            string
	}
	sources := []archiveSource{{
		url: req.Artifact.URL, sha: req.Artifact.SHA256, size: req.Artifact.Size,
		name: safeName + "-main", files: req.Artifact.Files, raw: req.Artifact.Raw,
	}}
	var totalSize int64 = req.Artifact.Size
	for i, c := range req.Artifact.Components {
		sources = append(sources, archiveSource{
			url: c.URL, sha: c.SHA256, size: c.Size,
			name: fmt.Sprintf("%s-comp%d", safeName, i), files: c.Files,
		})
		totalSize += c.Size
	}

	archives := make([]string, 0, len(sources))
	var completed int64
	for i, src := range sources {
		archive, err := m.dl.DownloadToProgressSized(ctx, src.url, src.sha, cacheDir, src.name, src.size,
			func(p download.Progress) {
				job.set(func(s *Status) {
					s.Downloaded = completed + p.Downloaded
					if totalSize > 0 {
						s.Total = totalSize
					} else {
						s.Total = completed + p.Total
					}
				})
			})
		if err != nil {
			job.set(func(s *Status) {
				s.Error = err.Error()
				s.Canceled = ctx.Err() != nil
				s.Done = true
				s.Retryable = true
			})
			job.log(fmt.Sprintf("FAILED component %d: %s", i, err.Error()))
			m.finish(req.ID, job, nil, false)
			return
		}
		defer os.Remove(archive)
		if info, err := os.Stat(archive); err == nil {
			completed += info.Size()
		}
		archives = append(archives, archive)
		if err := ctx.Err(); err != nil {
			job.set(func(s *Status) { s.Error = err.Error(); s.Canceled = true; s.Done = true })
			m.finish(req.ID, job, nil, false)
			return
		}
	}

	job.set(func(s *Status) { s.Phase = "installing" })
	job.log("extracting " + strings.Join(archives, ", "))
	installSources := make([]download.Source, len(sources))
	for i, archive := range archives {
		installSources[i] = download.Source{Archive: archive, RawTo: sources[i].raw}
		for _, f := range sources[i].files {
			installSources[i].Files = append(installSources[i].Files, download.FileMapping{From: f.From, To: f.To})
		}
	}
	opts := []download.Option{}
	if len(req.Preserve) > 0 {
		opts = append(opts, download.WithPreserve(req.Preserve...))
	}
	if req.VerifyStaging != nil {
		opts = append(opts, download.WithVerify(req.VerifyStaging))
	}
	if req.MergeStaging != nil {
		opts = append(opts, download.WithAfterExtract(req.MergeStaging))
	}
	if err := download.Install(installSources, req.Dest, req.Expected, opts...); err != nil {
		job.set(func(s *Status) {
			s.Error = err.Error()
			s.Canceled = ctx.Err() != nil
			s.Done = true
			// Download and extraction failures are usually transient; a staged
			// verification failure is not (the artifact itself is wrong).
			s.Retryable = req.VerifyStaging == nil
		})
		job.log("FAILED: " + err.Error())
		m.finish(req.ID, job, nil, false)
		return
	}

	// Build marker is written only after a successful atomic swap.
	if err := os.WriteFile(filepath.Join(req.Dest, "build"), []byte(req.Artifact.Build+"\n"), 0o644); err != nil {
		job.set(func(s *Status) { s.Error = err.Error(); s.Done = true })
		m.finish(req.ID, job, nil, false)
		return
	}

	if m.cfg != nil && req.ConfigKey != "" {
		err := m.cfg.Update(func(c *config.Config) error {
			c.Installed[req.ConfigKey] = req.Artifact.Build
			return nil
		})
		if err != nil {
			job.set(func(s *Status) { s.Error = err.Error(); s.Done = true })
			m.finish(req.ID, job, nil, false)
			return
		}
	}
	if req.OnSuccess != nil {
		if err := req.OnSuccess(req.Dest); err != nil {
			job.set(func(s *Status) { s.Error = err.Error(); s.Done = true })
			job.log("setup failed: " + err.Error())
			m.finish(req.ID, job, nil, false)
			return
		}
	}
	job.log("installed")
	job.set(func(s *Status) { s.Done = true; s.Phase = "installing"; s.Downloaded = s.Total })
	m.jobDone()
	m.finish(req.ID, job, nil, true)
}

// RunRequest describes a job that performs local work instead of downloading a
// verified artifact. It is used for operations owned by another tool (nvm).
type RunRequest struct {
	ID        string
	Package   string
	Version   string
	Run       func(ctx context.Context, log func(string)) error
	OnSuccess func() error
}

// SetVersion updates the version reported by a running job once the real
// version is known (for example after `nvm install 26` resolves to v26.13.1).
func (j *Job) SetVersion(version string) {
	j.set(func(s *Status) { s.Version = version })
}

// Run starts a local-work job with the same lifecycle and observability as an
// install job. Progress is indeterminate: Total/Downloaded go 0 -> 1.
func (m *Manager) Run(parent context.Context, req RunRequest) (*Job, error) {
	if req.ID == "" || req.Run == nil {
		return nil, fmt.Errorf("job id and run function are required")
	}
	m.mu.Lock()
	if _, exists := m.running[req.ID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("an install for %s is already running", req.ID)
	}
	ctx, cancel := context.WithCancel(parent)
	job := &Job{status: Status{
		ID: req.ID, Package: req.Package, Version: req.Version,
		Phase: "installing", Total: 1,
	}}
	job.cancel = cancel
	m.running[req.ID] = job
	m.mu.Unlock()

	if err := os.MkdirAll(m.logDir, 0o755); err != nil {
		m.finish(req.ID, job, err, false)
		return nil, err
	}
	logPath := filepath.Join(m.logDir, logFileName(req.ID))
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		m.finish(req.ID, job, err, false)
		return nil, err
	}
	job.mu.Lock()
	job.logFile = lf
	job.mu.Unlock()

	go func() {
		defer cancel()
		defer func() {
			job.mu.Lock()
			if job.logFile != nil {
				job.logFile.Close()
			}
			job.mu.Unlock()
		}()
		job.log(fmt.Sprintf("run %s %s via external manager", req.Package, req.Version))
		if err := req.Run(ctx, job.log); err != nil {
			job.set(func(s *Status) { s.Error = err.Error(); s.Canceled = ctx.Err() != nil; s.Done = true })
			job.log("FAILED: " + err.Error())
			m.finish(req.ID, job, nil, false)
			return
		}
		if req.OnSuccess != nil {
			if err := req.OnSuccess(); err != nil {
				job.set(func(s *Status) { s.Error = err.Error(); s.Done = true })
				job.log("setup failed: " + err.Error())
				m.finish(req.ID, job, nil, false)
				return
			}
		}
		job.log("installed")
		job.set(func(s *Status) { s.Done = true; s.Downloaded = s.Total })
		m.jobDone()
		m.finish(req.ID, job, nil, true)
	}()
	return job, nil
}

func (m *Manager) finish(id string, job *Job, err error, success bool) {
	if err != nil {
		job.set(func(s *Status) { s.Error = err.Error(); s.Done = true })
	}
	m.mu.Lock()
	delete(m.running, id)
	if !success && job.req.ID != "" {
		m.lastReq[id] = job.req
	} else if success {
		delete(m.lastReq, id)
	}
	m.recent[id] = job.Status()
	if len(m.recentOrder) >= maxRecentJobs {
		oldest := m.recentOrder[0]
		m.recentOrder = m.recentOrder[1:]
		delete(m.recent, oldest)
	}
	// A finished id may already be in the LRU list (e.g. a retry of the same
	// job). Dropping the old entry keeps the ordering meaningful and stops the
	// list from evicting a still-usable status too early.
	for i, old := range m.recentOrder {
		if old == id {
			m.recentOrder = append(m.recentOrder[:i], m.recentOrder[i+1:]...)
			break
		}
	}
	m.recentOrder = append(m.recentOrder, id)
	m.mu.Unlock()
}

// Job returns a running job by id.
func (m *Manager) Job(id string) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.running[id]
	return j, ok
}

// JobStatus returns the status of a running or recently finished job. This is
// what the daemon's job action uses so fast installs can still be observed.
func (m *Manager) JobStatus(id string) (Status, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.running[id]; ok {
		return j.Status(), true
	}
	s, ok := m.recent[id]
	return s, ok
}

// SetOnDone installs a hook called after every successful job.
func (m *Manager) SetOnDone(fn func()) { m.onDone = fn }

func (m *Manager) jobDone() {
	if m.onDone != nil {
		m.onDone()
	}
}

// Retry restarts a failed install with the exact same request. The caller
// should check Status.Retryable first: verification failures need a corrected
// artifact rather than another download of the same bytes.
func (m *Manager) Retry(parent context.Context, id string) (*Job, error) {
	m.mu.Lock()
	req, ok := m.lastReq[id]
	st, hasStatus := m.recent[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no failed install to retry for %s", id)
	}
	if hasStatus && !st.Retryable {
		return nil, fmt.Errorf("failure for %s is not retryable: the artifact itself must be corrected", id)
	}
	return m.Start(parent, req)
}

// List returns the currently running jobs.
func (m *Manager) List() []Status {
	m.mu.Lock()
	ids := make([]string, 0, len(m.running))
	for id := range m.running {
		ids = append(ids, id)
	}
	jobs := make([]*Job, 0, len(ids))
	for _, id := range ids {
		jobs = append(jobs, m.running[id])
	}
	m.mu.Unlock()
	out := make([]Status, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, j.Status())
	}
	return out
}

// NeedsReplacement reports the corrected artifact that should replace a
// currently installed yanked build. The manifest keeps the good build in the
// primary field and lists the bad one(s) in yanked{build:reason}.
func NeedsReplacement(mf *manifest.Manifest, pkg, version, platform, installedBuild string) (manifest.Artifact, bool) {
	a, ok := mf.Get(pkg, version, platform)
	if !ok || a.SHA256 == "" || a.Build == installedBuild {
		return manifest.Artifact{}, false
	}
	if _, installedIsYanked := a.Yanked[installedBuild]; !installedIsYanked {
		return manifest.Artifact{}, false
	}
	if _, primaryYanked := a.IsYanked(); primaryYanked {
		return manifest.Artifact{}, false
	}
	return a, true
}
