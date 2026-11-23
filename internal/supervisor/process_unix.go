//go:build !windows

package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lair-sh/lair/internal/services"
)

type osProcess struct {
	cmd *exec.Cmd
	log *os.File
}

func (p *osProcess) Wait() error {
	err := p.cmd.Wait()
	if p.log != nil {
		_ = p.log.Close()
	}
	return err
}
func (p *osProcess) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// Signal targets the whole process group so children of shells and wrappers
// are stopped too. Falls back to the direct process when group signalling is
// unavailable.
func (p *osProcess) Signal(sig os.Signal) error {
	if p.cmd.Process == nil {
		return nil
	}
	if s, ok := sig.(syscall.Signal); ok {
		err := syscall.Kill(-p.cmd.Process.Pid, s)
		if err == nil || err == syscall.ESRCH {
			return nil
		}
	}
	return p.cmd.Process.Signal(sig)
}

// signalStop honours the spec's declared shutdown command/signal. PostgreSQL,
// for example, declares SIGINT (fast shutdown) and a pg_ctl -m fast command;
// sending plain SIGTERM would wait for all client connections to close and a
// `services stop` could hang on a busy site. Falls back to SIGTERM.
func signalStop(p Process, spec services.Spec) error {
	if len(spec.StopCmd) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, spec.StopCmd[0], spec.StopCmd[1:]...)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	sig := syscall.SIGTERM
	switch spec.StopSignal {
	case "SIGINT", "INT":
		sig = syscall.SIGINT
	case "SIGQUIT", "QUIT":
		sig = syscall.SIGQUIT
	case "SIGKILL", "KILL":
		sig = syscall.SIGKILL
	}
	return p.Signal(sig)
}

// OSRunner starts services in their own process groups with logs on disk.
type OSRunner struct{ LogDir string }

func (r *OSRunner) Start(spec services.Spec) (Process, error) {
	cmd := exec.Command(spec.Executable, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	proc := &osProcess{cmd: cmd}
	if r.LogDir != "" {
		if err := os.MkdirAll(r.LogDir, 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(filepath.Join(r.LogDir, spec.ID+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		cmd.Stdout = f
		cmd.Stderr = f
		proc.log = f
	}
	if err := cmd.Start(); err != nil {
		if proc.log != nil {
			_ = proc.log.Close()
		}
		return nil, fmt.Errorf("start %s: %w", spec.ID, err)
	}
	return proc, nil
}
