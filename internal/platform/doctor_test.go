package platform

import (
	"strings"
	"syscall"
	"testing"
)

func TestParseHosts(t *testing.T) {
	ips := parseHosts("127.0.0.1 lair.test\n127.0.0.1  lair.test\n::1 localhost\n")
	if len(ips) != 2 || ips[0] != "127.0.0.1" || ips[1] != "::1" {
		t.Fatalf("ips = %v", ips)
	}
}

func TestDNSAdvice(t *testing.T) {
	if got := DNSAdvice("/home/u/.local/share/lair/setup-privileged.sh", "test", []string{"127.0.0.1"}, nil); got != "" {
		t.Fatalf("unexpected advice: %s", got)
	}
	got := DNSAdvice("/tmp/setup.sh", "test", nil, syscall.ECONNREFUSED)
	for _, want := range []string{"lair.test", "sudo /tmp/setup.sh", "getent hosts lair.test"} {
		if !strings.Contains(got, want) {
			t.Errorf("advice missing %q: %s", want, got)
		}
	}
	got = DNSAdvice("/tmp/setup.sh", "acme", []string{"1.2.3.4"}, nil)
	if !strings.Contains(got, "lair.acme") || !strings.Contains(got, "sudo /tmp/setup.sh") {
		t.Fatalf("advice = %s", got)
	}
}

func TestWebPortErrorIsActionable(t *testing.T) {
	denied := WebPortError("127.0.0.1:80", syscall.EACCES)
	for _, want := range []string{"127.0.0.1:80", "setcap", "cap_net_bind_service", "Configuración de Lair"} {
		if !strings.Contains(denied.Error(), want) {
			t.Errorf("permission advice missing %q: %s", want, denied)
		}
	}
	busy := WebPortError("127.0.0.1:443", syscall.EADDRINUSE)
	if !strings.Contains(busy.Error(), "puerto ocupado") || strings.Contains(busy.Error(), "setcap") {
		t.Fatalf("busy advice = %s", busy)
	}
}
