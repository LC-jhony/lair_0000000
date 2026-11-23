package web

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouterMatch(t *testing.T) {
	r := NewRouter()
	if err := r.Add(Site{Name: "acme.test", Root: "/srv/acme", PHPSocket: "/run/fpm-acme.sock"}); err != nil {
		t.Fatal(err)
	}
	s, name, ok := r.Match("acme.test:443")
	if !ok || name != "acme.test" || s.Root != "/srv/acme" {
		t.Fatalf("exact match = %+v %s %v", s, name, ok)
	}
	s, name, ok = r.Match("www.acme.test")
	if !ok || name != "www.acme.test" || s.Name != "acme.test" {
		t.Fatalf("subdomain match = %+v %s %v", s, name, ok)
	}
	if _, _, ok := r.Match("acme.evil"); ok {
		t.Fatal("unrelated host matched")
	}
	r.Remove("acme.test")
	if len(r.List()) != 0 {
		t.Fatal("site not removed")
	}
	if err := r.Add(Site{Name: "x"}); err == nil {
		t.Fatal("incomplete site accepted")
	}
}

func TestFastCGIParams(t *testing.T) {
	site := Site{Name: "acme.test", Root: "/srv/acme", PHPSocket: "/run/fpm.sock"}
	req := httptest.NewRequest(http.MethodPost, "http://acme.test/foo?a=1", strings.NewReader("x=1"))
	p := FastCGIParams(site, "www.acme.test", req)
	if p["SCRIPT_FILENAME"] != filepath.Join("/srv/acme", "public", "index.php") {
		t.Fatalf("SCRIPT_FILENAME = %s", p["SCRIPT_FILENAME"])
	}
	if p["SCRIPT_NAME"] != "foo" || p["REQUEST_METHOD"] != "POST" || p["QUERY_STRING"] != "a=1" {
		t.Fatalf("params = %+v", p)
	}
	if p["SERVER_NAME"] != "www.acme.test" || p["SERVER_PORT"] != "80" {
		t.Fatalf("server params = %+v", p)
	}
	req2 := httptest.NewRequest(http.MethodGet, "http://acme.test/style.css", nil)
	p2 := FastCGIParams(site, "acme.test", req2)
	if !strings.HasSuffix(p2["SCRIPT_FILENAME"], filepath.Join("public", "style.css")) {
		t.Fatalf("static script = %s", p2["SCRIPT_FILENAME"])
	}
}

func TestHandlerProxiesToFPM(t *testing.T) {
	// Fake unix listener acting as PHP-FPM.
	dir := t.TempDir()
	sock := filepath.Join(dir, "fpm.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Read until empty STDIN and reply with an HTTP response.
		for {
			typ, _, err := readRec(conn)
			if err != nil || typ == 5 { // stdinRecord
				break
			}
		}
		resp := []byte("Status: 200 OK\r\nContent-Type: application/json\r\n\r\n{\"route\":\"ok\"}")
		writeRec(conn, 6, resp) // stdoutRecord
		end := make([]byte, 8)
		end[4] = 0
		writeRec(conn, 3, end) // endRequest
		conn.Close()
	}()

	s := NewServer(filepath.Join(dir, "certs"), "test")
	if err := s.Router.Add(Site{Name: "acme.test", Root: "/srv/acme", PHPSocket: sock}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://acme.test/foo", nil)
	req.Host = "acme.test"
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != 200 || strings.TrimSpace(rr.Body.String()) != `{"route":"ok"}` {
		t.Fatalf("response = %d %q", rr.Code, rr.Body.String())
	}
}

func TestGetCertificate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs")
	s := NewServer(dir, "test")
	if err := s.Router.Add(Site{Name: "acme.test", Root: "/srv/acme", PHPSocket: "/run/fpm.sock"}); err != nil {
		t.Fatal(err)
	}
	cert, err := s.getCertificate(&tls.ClientHelloInfo{ServerName: "www.acme.test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("empty certificate")
	}
	// Only the canonical site name may appear in the issued certificate.
	if _, err := os.Stat(filepath.Join(dir, "acme.test.pem")); err != nil {
		t.Fatalf("canonical cert not persisted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "www.acme.test.pem")); !os.IsNotExist(err) {
		t.Fatalf("cert issued for the SNI subdomain instead of the site: %v", err)
	}
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range parsed.DNSNames {
		if strings.Contains(name, "www.acme.test") {
			t.Fatalf("SAN leaked SNI host: %v", parsed.DNSNames)
		}
	}
	if _, err := s.getCertificate(&tls.ClientHelloInfo{ServerName: "evil.example"}); err == nil {
		t.Fatal("issued cert for non-test domain")
	}
}

func TestGetCertificateRejectsUnparkedSNI(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs")
	s := NewServer(dir, "test")
	// Host is inside the domain but not parked: TLS must not mint a cert.
	if _, err := s.getCertificate(&tls.ClientHelloInfo{ServerName: "ghost.test"}); err == nil {
		t.Fatal("issued cert for unparked host")
	}
	if _, err := os.Stat(filepath.Join(dir, "ghost.test.pem")); !os.IsNotExist(err) {
		t.Fatalf("unparked host has a certificate: %v", err)
	}
	// After parking, the same SNI works and is cached.
	if err := s.Router.Add(Site{Name: "ghost.test", Root: "/srv/ghost", PHPSocket: "/run/fpm.sock"}); err != nil {
		t.Fatal(err)
	}
	first, err := s.getCertificate(&tls.ClientHelloInfo{ServerName: "ghost.test"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.getCertificate(&tls.ClientHelloInfo{ServerName: "ghost.test"})
	if err != nil || first != second {
		t.Fatalf("certificate was not cached: %v", err)
	}
	// Unparking revokes it even for a warm cache.
	s.Router.Remove("ghost.test")
	if _, err := s.getCertificate(&tls.ClientHelloInfo{ServerName: "ghost.test"}); err == nil {
		t.Fatal("cached cert served after unlink")
	}
}

func TestSafeStaticFileRejectsEscapingSymlinks(t *testing.T) {
	root := t.TempDir()
	docRoot := filepath.Join(root, "public")
	if err := os.MkdirAll(filepath.Join(docRoot, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(docRoot, "inside.txt")
	outside := filepath.Join(root, "secret.txt")
	os.WriteFile(inside, []byte("inside"), 0o644)
	os.WriteFile(outside, []byte("SECRET"), 0o644)

	if _, ok := safeStaticFile(docRoot, "inside.txt"); !ok {
		t.Fatal("regular file inside docRoot rejected")
	}
	if err := os.Symlink(outside, filepath.Join(docRoot, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if _, ok := safeStaticFile(docRoot, "escape.txt"); ok {
		t.Fatal("symlink escaping docRoot accepted")
	}
	if err := os.Symlink(inside, filepath.Join(docRoot, "sub", "ok.txt")); err != nil {
		t.Fatal(err)
	}
	if _, ok := safeStaticFile(docRoot, "sub/ok.txt"); !ok {
		t.Fatal("symlink inside docRoot rejected")
	}
	if _, ok := safeStaticFile(docRoot, "../secret.txt"); ok {
		t.Fatal("traversal accepted")
	}
}

func TestPHPFStderrIsNotServed(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "fpm.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			typ, _, err := readRec(conn)
			if err != nil || typ == 5 {
				break
			}
		}
		writeRec(conn, 6, []byte("Status: 200 OK\r\nContent-Type: text/plain\r\n\r\nOK"))
		writeRec(conn, 7, []byte("DB_PASSWORD=super-secret-stack-trace"))
		end := make([]byte, 8)
		writeRec(conn, 3, end)
	}()
	s := NewServer(filepath.Join(dir, "certs"), "test")
	s.Router.Add(Site{Name: "acme.test", Root: "/srv/acme", PHPSocket: sock})
	req := httptest.NewRequest(http.MethodGet, "http://acme.test/index.php", nil)
	req.Host = "acme.test"
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != 200 || rr.Body.String() != "OK" {
		t.Fatalf("response = %d %q", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "super-secret") {
		t.Fatalf("PHP-FPM stderr leaked: %q", rr.Body.String())
	}
}

func TestServerUsesConfiguredBind(t *testing.T) {
	s := NewServer(t.TempDir(), "test")
	s.HTTPPort, s.HTTPSPort = 18080, 18443
	s.Bind = "0.0.0.0"
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())
	if s.httpServer.Addr != "0.0.0.0:18080" || s.httpsServer.Addr != "0.0.0.0:18443" {
		t.Fatalf("addrs = %q / %q", s.httpServer.Addr, s.httpsServer.Addr)
	}
	s.Bind = "not-an-address"
	if err := s.Start(); err == nil {
		t.Fatal("invalid bind address accepted")
	}
}

func TestPortsInUse(t *testing.T) {
	// Just assert the helper runs; actual conflict detection is covered in the
	// services package test.
	if _, err := PortsInUse(); err != nil {
		t.Fatal(err)
	}
}

// Minimal FastCGI record helpers for the fake responder (kept local so the
// web package tests do not depend on fastcgi internals).
func readRec(conn net.Conn) (byte, []byte, error) {
	h := make([]byte, 8)
	if _, err := ioReadFull(conn, h); err != nil {
		return 0, nil, err
	}
	length := int(binary.BigEndian.Uint16(h[4:6]))
	pad := int(h[6])
	data := make([]byte, length)
	if _, err := ioReadFull(conn, data); err != nil {
		return 0, nil, err
	}
	if pad > 0 {
		skip := make([]byte, pad)
		ioReadFull(conn, skip)
	}
	return h[1], data, nil
}

func writeRec(conn net.Conn, typ byte, data []byte) error {
	pad := (8 - len(data)%8) % 8
	h := make([]byte, 8)
	h[0], h[1] = 1, typ
	binary.BigEndian.PutUint16(h[2:4], 1)
	binary.BigEndian.PutUint16(h[4:6], uint16(len(data)))
	h[6] = byte(pad)
	if _, err := conn.Write(h); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := conn.Write(data); err != nil {
			return err
		}
	}
	if pad > 0 {
		_, _ = conn.Write(make([]byte, pad))
	}
	return nil
}

func ioReadFull(r net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
