package platform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ResolveHost resolves a host the same way the privileged script verifies it:
// getent first (so systemd-resolved and nsswitch are honoured), Go resolver as
// a fallback when getent is unavailable.
func ResolveHost(host string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "getent", "hosts", host).Output()
	if err == nil {
		return parseHosts(string(out)), nil
	}
	return net.DefaultResolver.LookupHost(ctx, host)
}

func parseHosts(text string) []string {
	seen := map[string]bool{}
	var ips []string
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		ips = append(ips, fields[0])
	}
	return ips
}

// DNSAdvice returns an actionable message when the local domain does not
// resolve to 127.0.0.1. An empty string means the check passed.
func DNSAdvice(scriptPath, domain string, ips []string, err error) string {
	host := "lair." + strings.TrimPrefix(strings.TrimSpace(domain), ".")
	if host == "lair." {
		host = "lair.test"
	}
	if scriptPath == "" {
		scriptPath = "<LAIR_HOME>/setup-privileged.sh"
	}
	if err != nil {
		return fmt.Sprintf("DNS: %s no resuelve (%v). Solución: ejecuta sudo %s y comprueba con 'getent hosts %s'",
			host, err, scriptPath, host)
	}
	for _, ip := range ips {
		if ip == "127.0.0.1" {
			return ""
		}
	}
	return fmt.Sprintf("DNS: %s resuelve a %s, no a 127.0.0.1. Solución: ejecuta sudo %s "+
		"(instala el drop-in de systemd-resolved y el servicio lair-dns) o revisa otro resolver en 127.0.0.1:53535",
		host, strings.Join(ips, ", "), scriptPath)
}

// CheckWebPort probes a web listener without starting it. A failed probe
// carries an actionable message: missing CAP_NET_BIND_SERVICE or a port held
// by another program.
func CheckWebPort(port int, bind string) error {
	if bind == "" {
		bind = "127.0.0.1"
	}
	addr := net.JoinHostPort(bind, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return WebPortError(addr, err)
	}
	_ = ln.Close()
	return nil
}

// WebPortError classifies a bind error. It is exported so tests can feed
// synthetic errors without binding privileged ports.
func WebPortError(addr string, err error) error {
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("%s: permiso denegado; usa Configuración de Lair en la pestaña Sitios "+
			"con puertos altos o ejecuta sudo setcap cap_net_bind_service=+ep <binario-de-lair>", addr)
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("%s: puerto ocupado; ciérralo en el otro programa o usa Configuración de Lair en la pestaña Sitios", addr)
	}
	return fmt.Errorf("%s: %w", addr, err)
}
