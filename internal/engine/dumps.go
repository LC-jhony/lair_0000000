package engine

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lair-sh/lair/internal/config"
	"github.com/lair-sh/lair/internal/php"
)

// DumpsRequest is the wire payload of the `dumps` daemon action. `ingest` is
// internal-only: it is called by the generated PHP agent through the CLI.
type DumpsRequest struct {
	Op         string   `json:"op"`
	Types      []string `json:"types,omitempty"`
	ShowInPage bool     `json:"show_in_page,omitempty"`
	Site       string   `json:"site,omitempty"`
	Type       string   `json:"type,omitempty"`
	Time       string   `json:"time,omitempty"`
	Request    string   `json:"request,omitempty"`
	Message    string   `json:"message,omitempty"`
	Token      string   `json:"token,omitempty"`
}

// DumpsOptions is the persisted capture configuration.
type DumpsOptions struct {
	Queries    bool `json:"queries"`
	Logs       bool `json:"logs"`
	Views      bool `json:"views"`
	Jobs       bool `json:"jobs"`
	Requests   bool `json:"requests"`
	ShowInPage bool `json:"show_in_page"`
}

// DumpEvent is one line of a <site>.jsonl dump file.
type DumpEvent struct {
	ID      string `json:"id"`
	Site    string `json:"site"`
	Type    string `json:"type"`
	Time    string `json:"time"`
	Request string `json:"request,omitempty"`
	Message string `json:"message"`
}

// DumpsResult is returned by every dumps operation.
type DumpsResult struct {
	Capturing bool         `json:"capturing"`
	Options   DumpsOptions `json:"options"`
	Count     int          `json:"count"`
	Events    []DumpEvent  `json:"events,omitempty"`
	Output    string       `json:"output,omitempty"`
}

const (
	dumpsDirName   = "dumps"
	dumpsAgentName = "dumps-agent.php"
	dumpsINIFile   = "zz-lair-dumps.ini"
	dumpsTokenName = "dumps.token"
	dumpsMaxEvents = 500
)

var dumpTypes = []string{"queries", "logs", "views", "jobs", "requests"}

func dumpsOptions(c config.DumpsSettings) DumpsOptions {
	return DumpsOptions{
		Queries: c.Queries, Logs: c.Logs, Views: c.Views,
		Jobs: c.Jobs, Requests: c.Requests, ShowInPage: c.ShowInPage,
	}
}

// cleanDumpSite keeps an agent-supplied site name inside <root>/dumps. It also
// prevents JSONL files from escaping the data root via ".." or separators.
func cleanDumpSite(site string) (string, error) {
	site = strings.TrimSpace(site)
	if site == "" {
		site = "default"
	}
	if site == "." || site == ".." || strings.ContainsAny(site, `/\`) || strings.Contains(site, "..") {
		return "", fmt.Errorf("invalid dump site %q", site)
	}
	return site, nil
}

func (e *Engine) dumpsPath() string { return filepath.Join(e.Paths.Root, dumpsDirName) }

func newDumpID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("dump-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("dump-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// DumpsControl implements the `dumps` daemon action.
func (e *Engine) DumpsControl(ctx context.Context, req DumpsRequest) (DumpsResult, error) {
	var out DumpsResult
	switch req.Op {
	case "", "status", "capture-on", "capture-off", "options", "list", "clear", "ingest":
	default:
		return out, fmt.Errorf("unknown dumps op %q", req.Op)
	}

	c, err := e.Config.View()
	if err != nil {
		return out, fmt.Errorf("dumps config: %w", err)
	}

	switch req.Op {
	case "capture-on", "capture-off", "options":
		next := c.Dumps
		switch req.Op {
		case "capture-on":
			next.Capturing = true
		case "capture-off":
			next.Capturing = false
		case "options":
			if len(req.Types) > 0 {
				selected := map[string]bool{}
				for _, t := range req.Types {
					for _, known := range dumpTypes {
						if strings.EqualFold(t, known) {
							selected[known] = true
						}
					}
				}
				next.Queries, next.Logs, next.Views = selected["queries"], selected["logs"], selected["views"]
				next.Jobs, next.Requests = selected["jobs"], selected["requests"]
			}
			next.ShowInPage = req.ShowInPage
		}
		if err := e.Config.Update(func(cfg *config.Config) error {
			cfg.Dumps = next
			return nil
		}); err != nil {
			return out, fmt.Errorf("save dumps settings: %w", err)
		}
		c.Dumps = next
		if err := e.syncDumpsAgent(next.Capturing); err != nil {
			return out, fmt.Errorf("sync dumps agent: %w", err)
		}
		// PHP-FPM reads php.ini/scan dirs at process start. Restart the pools
		// so the new auto_prepend_file is actually loaded by the running site.
		e.syncFPM()
		out.Output = map[string]string{
			"capture-on": "Captura activada", "capture-off": "Captura desactivada",
			"options": "Opciones actualizadas",
		}[req.Op]
	case "ingest":
		event, err := e.ingestDump(req)
		if err != nil {
			return out, fmt.Errorf("ingest dump: %w", err)
		}
		out.Output = "Evento guardado (" + event.ID + ")"
	case "clear":
		if err := e.clearDumps(); err != nil {
			return out, fmt.Errorf("clear dumps: %w", err)
		}
		out.Output = "Dumps vaciados"
	case "list":
		events, err := e.listDumps(req)
		if err != nil {
			return out, fmt.Errorf("list dumps: %w", err)
		}
		out.Events, out.Count = events, len(events)
	}

	if req.Op != "list" {
		count, err := e.countDumps()
		if err != nil {
			return out, fmt.Errorf("count dumps: %w", err)
		}
		out.Count = count
	}
	out.Capturing, out.Options = c.Dumps.Capturing, dumpsOptions(c.Dumps)
	_ = ctx // persistence is local and short; ctx is reserved for future remote stores
	return out, nil
}

func (e *Engine) ingestDump(req DumpsRequest) (DumpEvent, error) {
	site, err := cleanDumpSite(req.Site)
	if err != nil {
		return DumpEvent{}, err
	}
	typ := strings.ToLower(strings.TrimSpace(req.Type))
	if typ == "" {
		typ = "log"
	}
	when := strings.TrimSpace(req.Time)
	if when == "" {
		when = time.Now().UTC().Format(time.RFC3339)
	}
	event := DumpEvent{
		ID: newDumpID(), Site: site, Type: typ, Time: when,
		Request: req.Request, Message: req.Message,
	}
	line, err := json.Marshal(event)
	if err != nil {
		return DumpEvent{}, fmt.Errorf("encode event: %w", err)
	}
	e.dumpsMu.Lock()
	defer e.dumpsMu.Unlock()
	if err := os.MkdirAll(e.dumpsPath(), 0o700); err != nil {
		return DumpEvent{}, fmt.Errorf("create dumps dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(e.dumpsPath(), site+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return DumpEvent{}, fmt.Errorf("open dump file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return DumpEvent{}, fmt.Errorf("append event: %w", err)
	}
	return event, nil
}

func (e *Engine) allDumps() ([]DumpEvent, error) {
	entries, err := os.ReadDir(e.dumpsPath())
	if os.IsNotExist(err) {
		return []DumpEvent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read dumps dir: %w", err)
	}
	events := []DumpEvent{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		f, err := os.Open(filepath.Join(e.dumpsPath(), entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", entry.Name(), err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var ev DumpEvent
			if err := json.Unmarshal([]byte(line), &ev); err == nil {
				events = append(events, ev)
			}
		}
		readErr := scanner.Err()
		_ = f.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), readErr)
		}
	}
	return events, nil
}

func (e *Engine) countDumps() (int, error) {
	events, err := e.allDumps()
	if err != nil {
		return 0, err
	}
	return len(events), nil
}

func (e *Engine) listDumps(req DumpsRequest) ([]DumpEvent, error) {
	events, err := e.allDumps()
	if err != nil {
		return nil, err
	}
	filtered := make([]DumpEvent, 0, len(events))
	for _, ev := range events {
		if req.Site != "" && !strings.EqualFold(ev.Site, req.Site) {
			continue
		}
		if req.Type != "" && !strings.EqualFold(ev.Type, req.Type) {
			continue
		}
		if req.Time != "" && !strings.Contains(ev.Time, req.Time) {
			continue
		}
		if req.Request != "" && !strings.Contains(strings.ToLower(ev.Request), strings.ToLower(req.Request)) {
			continue
		}
		if req.Message != "" && !strings.Contains(strings.ToLower(ev.Message), strings.ToLower(req.Message)) {
			continue
		}
		filtered = append(filtered, ev)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Time > filtered[j].Time })
	if len(filtered) > dumpsMaxEvents {
		filtered = filtered[len(filtered)-dumpsMaxEvents:]
	}
	return filtered, nil
}

func (e *Engine) clearDumps() error {
	e.dumpsMu.Lock()
	defer e.dumpsMu.Unlock()
	entries, err := os.ReadDir(e.dumpsPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read dumps dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		if err := os.Remove(filepath.Join(e.dumpsPath(), entry.Name())); err != nil {
			return fmt.Errorf("remove %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// EnsureDumpsAgent writes the PHP agent and its dedicated token, and wires
// auto_prepend_file into the CLI scan dir of every installed PHP. FPM scan
// dirs are deliberately never touched.
func (e *Engine) EnsureDumpsAgent() error {
	c, err := e.Config.View()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return e.syncDumpsAgent(c.Dumps.Capturing)
}

func (e *Engine) syncDumpsAgent(enabled bool) error {
	if err := os.MkdirAll(e.Paths.Tools, 0o755); err != nil {
		return fmt.Errorf("tools dir: %w", err)
	}
	if err := os.MkdirAll(e.Paths.Run, 0o700); err != nil {
		return fmt.Errorf("run dir: %w", err)
	}
	tokenPath := filepath.Join(e.Paths.Run, dumpsTokenName)
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return fmt.Errorf("dumps token: %w", err)
		}
		if err := os.WriteFile(tokenPath, []byte(hex.EncodeToString(buf)+"\n"), 0o600); err != nil {
			return fmt.Errorf("write dumps token: %w", err)
		}
		_ = os.Chmod(tokenPath, 0o600)
	} else if err != nil {
		return fmt.Errorf("dumps token: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("lair executable: %w", err)
	}
	agent := dumpsAgentSource(exe, e.Paths.Root, tokenPath)
	agentPath := filepath.Join(e.Paths.Tools, dumpsAgentName)
	if err := os.WriteFile(agentPath, []byte(agent), 0o644); err != nil {
		return fmt.Errorf("write dumps agent: %w", err)
	}
	_ = os.Chmod(agentPath, 0o644)

	installed, _ := php.Installed(e.Paths)
	for _, info := range installed {
		dir := filepath.Join(info.Dir, "conf.d-cli")
		iniPath := filepath.Join(dir, dumpsINIFile)
		if !enabled {
			if err := os.Remove(iniPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove dumps ini: %w", err)
			}
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("php cli scan dir: %w", err)
		}
		body := fmt.Sprintf("; Generated by Lair. Do not edit.\nauto_prepend_file=%s\n", strconv.Quote(agentPath))
		if err := os.WriteFile(iniPath, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write dumps ini: %w", err)
		}
	}
	return nil
}

// dumpsAgentSource returns a self-contained PHP preload script. All values
// are injected as PHP single-quoted literals so no shell is involved; the
// agent itself uses proc_open with an argv array, which keeps spaces and
// quotes in captured content out of the command line.
func dumpsAgentSource(binary, home, tokenPath string) string {
	phpQuote := func(s string) string {
		return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `'`, `\'`) + "'"
	}
	return fmt.Sprintf(`<?php
// Generated by Lair. Captures PHP output, dump() and log events into
// %s. Safe to delete: it is regenerated on `+"`lair dumps capture-on`"+`.
$lairBin  = %s;
$lairHome = %s;
$lairToken = %s;

if (!function_exists('lair_dumps_send')) {
    function lair_dumps_send($type, $message, $request = '')
    {
        global $lairBin, $lairHome, $lairToken;
        $site = getenv('LAIR_DUMPS_SITE');
        if (!$site) { $site = getenv('LAIR_SITE'); }
        if (!$site) { $site = basename((string) getcwd()); }
        $argv = array(
            $lairBin, 'dumps', 'ingest',
            '--site', (string) $site,
            '--type', (string) $type,
            '--time', gmdate('c'),
            '--request', (string) $request,
            '--message', (string) $message,
            '--token', trim((string) @file_get_contents($lairToken)),
        );
        $cmd = PHP_VERSION_ID >= 70400 ? $argv : implode(' ', array_map('escapeshellarg', $argv));
        $desc = array(0 => array('pipe', 'r'), 1 => array('pipe', 'w'), 2 => array('pipe', 'w'));
        $pipes = array();
        // FPM does not inherit the daemon's LAIR_HOME. Pass it explicitly so
        // the child talks to the same data root and dumps token.
        $env = array('LAIR_HOME' => $lairHome);
        $proc = @proc_open($cmd, $desc, $pipes, null, $env);
        if (!is_resource($proc)) { return; }
        fclose($pipes[0]);
        stream_get_contents($pipes[1]);
        stream_get_contents($pipes[2]);
        fclose($pipes[1]);
        fclose($pipes[2]);
        proc_close($proc);
    }
}

// Symfony VarDumper covers Laravel's dump() helper when it is installed.
try {
    if (class_exists('Symfony\Component\VarDumper\VarDumper')) {
        \Symfony\Component\VarDumper\VarDumper::setHandler(function ($var) {
            $text = \Symfony\Component\VarDumper\VarDumper::export($var);
            $req = isset($_SERVER['REQUEST_URI']) ? (string) $_SERVER['REQUEST_URI'] : '';
            lair_dumps_send('views', $text, $req);
        });
    }
} catch (\Throwable $e) {
}

// Capture ordinary PHP output without changing what the script prints.
try {
    ob_start(function ($buffer) {
        if (trim((string) $buffer) !== '') {
            $req = isset($_SERVER['REQUEST_URI']) ? (string) $_SERVER['REQUEST_URI'] : '';
            lair_dumps_send('logs', $buffer, $req);
        }
        return $buffer;
    }, 8192);
    register_shutdown_function(function () {
        while (ob_get_level() > 0) { @ob_end_flush(); }
    });
} catch (\Throwable $e) {
}

// PHP notices/warnings are useful dumps too. Returning false preserves the
// engine's normal error handling.
try {
    set_error_handler(function ($severity, $message, $file, $line) {
        lair_dumps_send('logs', $message . ' at ' . $file . ':' . $line, '');
        return false;
    });
} catch (\Throwable $e) {
}

// Best-effort Laravel hook: this class is only available once the framework
// has booted, so failure here is intentionally silent.
try {
    if (class_exists('Illuminate\Support\Facades\Event')) {
        \Illuminate\Support\Facades\Event::listen('Illuminate\Log\Events\MessageLogged', function ($event) {
            lair_dumps_send('logs', (string) (isset($event->message) ? $event->message : ''), '');
        });
    }
} catch (\Throwable $e) {
}
`, filepath.Join(home, dumpsDirName), phpQuote(binary), phpQuote(home), phpQuote(tokenPath))
}
