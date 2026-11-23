package pki

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestEnsureCAIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs")
	cert1, key1, err := EnsureCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cert1.IsCA || cert1.Subject.CommonName != caCommonName {
		t.Fatalf("not a CA: %+v", cert1.Subject)
	}
	cert2, key2, err := EnsureCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cert1.SerialNumber.Cmp(cert2.SerialNumber) != 0 || key1.D.Cmp(key2.D) != 0 {
		t.Fatal("CA changed on second EnsureCA")
	}
	// Key must be 0600.
	_, keyPath := Paths(dir)
	info, _ := os.Stat(keyPath)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("CA key mode = %o", info.Mode().Perm())
	}
}

func TestIssueSiteAndVerifyChain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs")
	caCert, _, err := EnsureCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath, err := IssueSite(dir, "acme.test", "acme.test", "*.acme.test")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, _ := os.ReadFile(certPath)
	keyPEM, _ := os.ReadFile(keyPath)
	block, _ := pem.Decode(certPEM)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "www.acme.test", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		t.Fatalf("leaf did not verify: %v", err)
	}
	if len(leaf.DNSNames) != 2 {
		t.Fatalf("SANs = %v", leaf.DNSNames)
	}
	if leaf.NotAfter.Sub(time.Now()) < 300*24*time.Hour {
		t.Fatal("leaf validity too short")
	}
	if len(keyPEM) == 0 {
		t.Fatal("empty key")
	}
	if info, _ := os.Stat(keyPath); info.Mode().Perm() != 0o600 {
		t.Fatalf("leaf key mode = %o", info.Mode().Perm())
	}
}

func TestIssueSiteDefaultSANs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs")
	certPath, _, err := IssueSite(dir, "blog.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "blog.test.pem")); err != nil {
		t.Fatalf("unexpected cert path %s: %v", certPath, err)
	}
	data, _ := os.ReadFile(certPath)
	block, _ := pem.Decode(data)
	leaf, _ := x509.ParseCertificate(block.Bytes)
	if len(leaf.DNSNames) != 2 || leaf.DNSNames[1] != "*.blog.test" {
		t.Fatalf("default SANs = %v", leaf.DNSNames)
	}
}

func TestParseCARejectsGarbage(t *testing.T) {
	if _, _, err := ParseCA([]byte("nope"), []byte("nope")); err == nil {
		t.Fatal("garbage CA accepted")
	}
}

func TestEnsureCAConcurrent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs")
	const n = 16
	serials := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cert, _, err := EnsureCA(dir)
			errs[i] = err
			if cert != nil {
				serials[i] = cert.SerialNumber.String()
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("EnsureCA %d: %v", i, err)
		}
		if serials[i] != serials[0] {
			t.Fatalf("concurrent callers got different CAs: %v", serials)
		}
	}
}

func TestEnsureCABundleContainsLocalCA(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs")
	cert, _, err := EnsureCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(BundlePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if !bytes.Contains(data, certPEM) {
		t.Fatal("ca-bundle.pem does not contain the local CA")
	}
	info, _ := os.Stat(BundlePath(dir))
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("bundle mode = %o", info.Mode().Perm())
	}
}

func TestParseCARejectsMismatchedKey(t *testing.T) {
	certDir := filepath.Join(t.TempDir(), "certs")
	keyDir := filepath.Join(t.TempDir(), "other-certs")
	if _, _, err := EnsureCA(certDir); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureCA(keyDir); err != nil {
		t.Fatal(err)
	}
	certPEM, err := os.ReadFile(certPath(certDir))
	if err != nil {
		t.Fatal(err)
	}
	foreignKey, err := os.ReadFile(keyPath(keyDir))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseCA(certPEM, foreignKey); err == nil {
		t.Fatal("CA certificate accepted a key from another CA")
	}
}

func certPath(dir string) string {
	p, _ := Paths(dir)
	return p
}

func keyPath(dir string) string {
	_, p := Paths(dir)
	return p
}

func TestSANsForCanonicalOnly(t *testing.T) {
	got := SANsFor("www.acme.test", "test")
	want := []string{"www.acme.test", "*.www.acme.test"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("SANsFor = %v, want %v", got, want)
	}
	if SANsFor("evil.example", "test") != nil {
		t.Fatal("host outside the domain was accepted")
	}
	if SANsFor("bad name.test", "test") != nil {
		t.Fatal("host with whitespace was accepted")
	}
	if NormalizeHost("ACME.test.") != "acme.test" {
		t.Fatal("host normalisation failed")
	}
}

var _ = net.ParseIP
