package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lair-sh/lair/internal/daemon"
)

// confirmFlag removes the explicit-confirmation flags from a subcommand's
// arguments. Destructive clear operations never run on a bare command.
func confirmFlag(args []string) ([]string, bool) {
	rest := make([]string, 0, len(args))
	confirm := false
	for _, arg := range args {
		switch arg {
		case "--yes", "-y", "--confirm":
			confirm = true
		default:
			rest = append(rest, arg)
		}
	}
	return rest, confirm
}

// dumpsFlags parses the argv-safe form used by the generated PHP agent: every
// value is a separate argument, so no shell quoting is involved.
func dumpsFlags(args []string) (daemon.DumpsRequest, error) {
	var req daemon.DumpsRequest
	for i := 0; i < len(args); i++ {
		name := args[i]
		if !strings.HasPrefix(name, "--") {
			return req, fmt.Errorf("unexpected dumps argument %q", name)
		}
		if i+1 >= len(args) {
			return req, fmt.Errorf("%s requires a value", name)
		}
		value := args[i+1]
		i++
		switch name {
		case "--site":
			req.Site = value
		case "--request-id":
			req.RequestID = value
		case "--duration-ms":
			ms, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return req, fmt.Errorf("--duration-ms: %w", err)
			}
			req.DurationMS = ms
		case "--type":
			req.Type = value
		case "--time":
			req.Time = value
		case "--request":
			req.Request = value
		case "--message":
			req.Message = value
		case "--token":
			req.Token = value
		default:
			return req, fmt.Errorf("unknown dumps flag %q", name)
		}
	}
	return req, nil
}

func (c *ctx) dumps(args []string) error {
	op := "status"
	var req daemon.DumpsRequest
	if len(args) > 0 {
		op = args[0]
		args = args[1:]
	}
	var confirm bool
	args, confirm = confirmFlag(args)
	switch op {
	case "status", "list", "clear", "capture-on", "capture-off":
		req.Op = op
		if op == "clear" {
			req.Confirm = confirm
		}
	case "options":
		req.Op = op
		// Usage: lair dumps options queries,logs,views [show-in-page]
		if len(args) > 0 {
			req.Types = strings.Split(args[0], ",")
			args = args[1:]
		}
		if len(args) > 0 && (args[0] == "show-in-page" || args[0] == "on") {
			req.ShowInPage = true
		}
	case "ingest":
		parsed, err := dumpsFlags(args)
		if err != nil {
			return err
		}
		req = parsed
		// The dedicated token gates this internal action. It never travels
		// through a URL or an environment variable.
		data, err := os.ReadFile(filepath.Join(c.paths.Run, "dumps.token"))
		if err != nil {
			return fmt.Errorf("read dumps token: %w", err)
		}
		if strings.TrimSpace(req.Token) == "" || req.Token != strings.TrimSpace(string(data)) {
			return fmt.Errorf("invalid dumps token")
		}
		req.Op, req.Token = "ingest", ""
	default:
		return fmt.Errorf("unknown dumps subcommand %q", op)
	}
	var result daemon.DumpsResult
	if err := c.call(daemon.ActionDumps, req, &result); err != nil {
		return err
	}
	if result.Output != "" {
		c.printf("%s\n", result.Output)
	} else {
		c.printf("Captura: %v · eventos: %d\n", result.Capturing, result.Count)
	}
	if op == "list" {
		for _, ev := range result.Events {
			c.printf("%s [%s] %s %s\n", ev.Time, ev.Type, ev.Site, ev.Message)
		}
	}
	return nil
}

func (c *ctx) mail(args []string) error {
	op, id := "status", ""
	if len(args) > 0 {
		op = args[0]
		args = args[1:]
	}
	args, confirm := confirmFlag(args)
	req := daemon.MailRequest{Op: op}
	switch op {
	case "status", "start", "stop", "list":
	case "clear":
		req.Confirm = confirm
		if len(args) > 0 {
			req.Inbox = args[0]
		}
	case "message", "open", "raw":
		if len(args) < 1 {
			return fmt.Errorf("usage: lair mail %s <id>", op)
		}
		id = args[0]
		req.ID = id
		if op == "open" {
			req.Op = "message"
		}
	case "env":
		req.Op = "status"
	default:
		return fmt.Errorf("unknown mail subcommand %q", op)
	}
	var result daemon.MailResult
	if err := c.call(daemon.ActionMail, req, &result); err != nil {
		return err
	}
	if op == "env" {
		c.printf("%s", result.Env)
		return nil
	}
	if result.Output != "" {
		c.printf("%s\n", result.Output)
	}
	c.printf("SMTP: %v · puerto %d · mensajes: %d\n", result.Running, result.Port, result.Count)
	if op == "list" {
		for _, msg := range result.Messages {
			c.printf("%s  %s -> %s  %s\n", msg.ID, msg.From, msg.To, msg.Subject)
		}
	}
	if (op == "message" || op == "open") && result.Message != nil {
		buf, _ := json.MarshalIndent(result.Message, "", "  ")
		c.printf("%s\n", buf)
	}
	if op == "raw" {
		c.printf("%s", result.Raw)
	}
	return nil
}

func (c *ctx) debugger(args []string) error {
	op := "status"
	if len(args) > 0 {
		op = args[0]
	}
	var result daemon.DebuggerResult
	if err := c.call(daemon.ActionDebugger, daemon.DebuggerRequest{Op: op}, &result); err != nil {
		return err
	}
	c.printf("Detección: %v · breakpoints: %v · puerto %d · modo %s\n",
		result.Detect, result.Breakpoints, result.Port, result.Mode)
	for _, v := range result.Versions {
		state := "no disponible"
		if v.Available {
			state = "disponible"
		}
		c.printf("  PHP %s: Xdebug %s\n", v.Version, state)
	}
	return nil
}
