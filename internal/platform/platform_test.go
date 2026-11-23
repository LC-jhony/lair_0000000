package platform

import (
	"strings"
	"testing"
)

func TestGenerateUnit(t *testing.T) {
	if Current() != KindSystemd {
		t.Skip("systemd only on linux")
	}
	f, err := Generate(UnitConfig{
		User: "jhony", Home: "/home/jhony", DataDir: "/home/jhony/.local/share/lair",
		ExecPath: "/home/jhony/.local/share/lair/bin/lair", Domain: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"User=%i", "AmbientCapabilities=CAP_NET_BIND_SERVICE", "Restart=always",
		"ExecStart=/home/jhony/.local/share/lair/bin/lair daemon --foreground",
	} {
		if !strings.Contains(f.UnitContent, want) {
			t.Errorf("unit missing %q:\n%s", want, f.UnitContent)
		}
	}
	if !strings.Contains(f.DropInContent, "Environment=LAIR_HOME=") {
		t.Errorf("drop-in = %s", f.DropInContent)
	}
	if !strings.Contains(f.PolkitContent, "lair@") || !strings.Contains(f.PolkitContent, "subject.user") {
		t.Errorf("polkit = %s", f.PolkitContent)
	}
	if !strings.Contains(f.ResolvedContent, "Domains=~test") || !strings.Contains(f.ResolvedContent, "DNS=127.0.0.1:53535") {
		t.Errorf("resolved = %s", f.ResolvedContent)
	}
	for _, want := range []string{
		"port=53535", "listen-address=127.0.0.1", "bind-interfaces",
		"no-resolv", "no-hosts", "address=/test/127.0.0.1",
	} {
		if !strings.Contains(f.DnsmasqConfContent, want) {
			t.Errorf("dnsmasq.conf missing %q:\n%s", want, f.DnsmasqConfContent)
		}
	}
	if !strings.Contains(f.DNSUnitContent, "/usr/sbin/dnsmasq --keep-in-foreground --conf-file=") ||
		!strings.Contains(f.DNSUnitPath, "lair-dns.service") {
		t.Errorf("dns unit = %s / %s", f.DNSUnitPath, f.DNSUnitContent)
	}
	if !strings.Contains(f.FallbackResolvedContent, "DNS=127.0.0.1:53535") ||
		!strings.Contains(f.FallbackResolvedContent, "Domains=~test") {
		t.Errorf("fallback = %s", f.FallbackResolvedContent)
	}
	if !strings.HasSuffix(f.DropInPath, "lair@jhony.service.d/10-lair.conf") {
		t.Errorf("drop-in path = %s", f.DropInPath)
	}
}

func TestGenerateRejectsRelativePaths(t *testing.T) {
	if Current() != KindSystemd {
		t.Skip("systemd only on linux")
	}
	_, err := Generate(UnitConfig{User: "u", ExecPath: "relative", DataDir: "/data"})
	if err == nil {
		t.Fatal("relative exec path accepted")
	}
	_, err = Generate(UnitConfig{User: "", ExecPath: "/x", DataDir: "/data"})
	if err == nil {
		t.Fatal("missing user accepted")
	}
}

func TestPrivilegedPlanScript(t *testing.T) {
	if Current() != KindSystemd {
		t.Skip("systemd only on linux")
	}
	plan, err := BuildPlan(UnitConfig{
		User: "jhony", Home: "/home/jhony", DataDir: "/home/jhony/.local/share/lair",
		ExecPath: "/home/jhony/.local/share/lair/bin/lair",
	})
	if err != nil {
		t.Fatal(err)
	}
	script := plan.Script()
	for _, want := range []string{
		"/etc/systemd/system/lair@.service", "/etc/polkit-1/rules.d/50-lair.rules",
		"/etc/systemd/resolved.conf.d/50-lair.conf", "systemctl daemon-reload",
		"LAIR_EOF",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q", want)
		}
	}
	if len(plan.Actions) < 3 {
		t.Fatalf("expected multiple privileged actions, got %d", len(plan.Actions))
	}
}

func TestPrivilegedDNSScriptIsSafeAndTransactional(t *testing.T) {
	if Current() != KindSystemd {
		t.Skip("systemd only on linux")
	}
	plan, err := BuildPlan(UnitConfig{
		User: "jhony", Home: "/home/jhony", DataDir: "/home/jhony/.local/share/lair",
		ExecPath: "/home/jhony/.local/share/lair/bin/lair", Domain: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	script := plan.Script()
	for _, want := range []string{
		"port=53535", "listen-address=127.0.0.1", "bind-interfaces",
		"no-resolv", "no-hosts", "address=/test/127.0.0.1",
		"/usr/sbin/dnsmasq --keep-in-foreground --conf-file=",
		"/etc/systemd/system/lair-dns.service", "systemctl enable --now lair-dns.service",
		"DNS=127.0.0.1:53535", "Domains=~test",
		"sudo setcap cap_net_bind_service=+ep /home/jhony/.local/share/lair/bin/lair",
		"backup /etc/resolv.conf", "backup /etc/NetworkManager/NetworkManager.conf",
		"backup " + shQuote(ResolvedDropIn) + " 50-lair.conf",
		"rm -f /etc/dnsmasq.d/valet /etc/NetworkManager/conf.d/valet.conf /etc/NetworkManager/dnsmasq.d/valet",
		"getent hosts lair.test", "getent ahostsv4 example.com",
		"rollback()", "restore /etc/resolv.conf",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q", want)
		}
	}
	// The fallback must keep the modern resolved syntax and warn instead of
	// breaking public DNS when dnsmasq cannot bind the high port.
	for _, want := range []string{
		"command -v dnsmasq", "port_busy()", "DNS=127.0.0.1:53535",
		"Internet sigue funcionando", "exit 0",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("fallback missing %q", want)
		}
	}
	// The safe dnsmasq path must be reached only after the fallback check.
	if strings.Index(script, "port_busy()") > strings.Index(script, "systemctl enable --now lair-dns.service") {
		t.Error("dnsmasq service started before the availability/fallback check")
	}
	resolvedWrite := strings.Index(script, "cat > "+shQuote(ResolvedDropIn))
	if strings.Index(script, "backup /etc/resolv.conf") > resolvedWrite || resolvedWrite < 0 {
		t.Error("resolv.conf was modified before being backed up")
	}
}

func TestInstallTrustScripts(t *testing.T) {
	p := InstallTrust("/data/certs/ca.pem")
	for _, want := range []string{"update-ca-certificates", "update-ca-trust", "/data/certs/ca.pem"} {
		if !strings.Contains(p.SystemScript, want) {
			t.Errorf("system script missing %q", want)
		}
	}
	if !strings.Contains(p.UserScript, "certutil") || !strings.Contains(p.UserScript, "lair-local-ca") {
		t.Errorf("user script = %s", p.UserScript)
	}
	if !strings.Contains(p.SystemScript, "--remove") {
		t.Error("trust script has no removal path")
	}
	// Removal must not re-add the certificate first. The add and remove
	// commands live in separate helpers selected by ACTION below them.
	add := strings.Index(p.UserScript, "add_nss()")
	remove := strings.Index(p.UserScript, "remove_nss()")
	apply := strings.Index(p.UserScript, "apply()")
	if add < 0 || remove < 0 || apply < 0 || !(add < remove && remove < apply) {
		t.Fatalf("user script has no clean add/remove dispatch:\n%s", p.UserScript)
	}
	if !strings.Contains(p.UserScript[apply:], `"$ACTION" = "--remove"`) {
		t.Error("user script never checks --remove when dispatching")
	}
}

func TestCertPathsAndResolvedApplies(t *testing.T) {
	ca, key, cert, ck := CertPaths("/data/certs", "*.acme.test")
	if ca != "/data/certs/ca.pem" || key != "/data/certs/ca-key.pem" {
		t.Fatalf("ca paths = %s %s", ca, key)
	}
	if cert != "/data/certs/acme.test.pem" || ck != "/data/certs/acme.test-key.pem" {
		t.Fatalf("site paths = %s %s", cert, ck)
	}
	actions := ResolvedConfigApplies("", "test", 53535)
	if len(actions) != 2 || !strings.Contains(strings.Join(actions[0].Command, " "), "127.0.0.1#53535") {
		t.Fatalf("actions = %+v", actions)
	}
}
