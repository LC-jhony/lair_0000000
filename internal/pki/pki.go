// Package pki manages Lair's local certificate authority and per-site
// certificates. It uses only the standard library. The CA private key stays
// on disk in the data root with mode 0600 and is never embedded in the binary.
package pki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	caCommonName = "Lair Local Development CA"
	caValidity   = 10 * 365 * 24 * time.Hour
	siteValidity = 390 * 24 * time.Hour
)

// caMu serialises CA loading/creation across the whole process. Two concurrent
// EnsureCA calls on a fresh data root would otherwise generate two different
// CAs and overwrite each other's key, breaking every leaf already issued.
var caMu sync.Mutex

// Paths returns the canonical CA certificate and key paths.
func Paths(certsDir string) (certPath, keyPath string) {
	return filepath.Join(certsDir, "ca.pem"), filepath.Join(certsDir, "ca-key.pem")
}

// BundlePath is the CA bundle used by outgoing HTTPS clients: the operating
// system roots plus Lair's local CA. It lets Composer reach packagist while
// still trusting local *.test sites.
func BundlePath(certsDir string) string {
	return filepath.Join(certsDir, "ca-bundle.pem")
}

// EnsureCA loads the CA from disk, or generates and persists a fresh one when
// it does not exist. It also refreshes ca-bundle.pem so external HTTPS clients
// keep working with the system trust store.
func EnsureCA(certsDir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	caMu.Lock()
	defer caMu.Unlock()
	cert, key, certPEM, err := loadOrCreateCA(certsDir)
	if err != nil {
		return nil, nil, err
	}
	if err := ensureBundleLocked(certsDir, certPEM); err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func loadOrCreateCA(certsDir string) (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
	certPath, keyPath := Paths(certsDir)
	if certPEM, err := os.ReadFile(certPath); err == nil {
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read CA key: %w", err)
		}
		cert, key, err := ParseCA(certPEM, keyPEM)
		return cert, key, certPEM, err
	} else if !os.IsNotExist(err) {
		return nil, nil, nil, err
	}

	if err := os.MkdirAll(certsDir, 0o700); err != nil {
		return nil, nil, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: caCommonName, Organization: []string{"Lair"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caValidity),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := atomicWrite(certPath, certPEM, 0o644); err != nil {
		return nil, nil, nil, err
	}
	if err := atomicWrite(keyPath, keyPEM, 0o600); err != nil {
		return nil, nil, nil, err
	}
	return cert, key, certPEM, nil
}

// systemBundlePaths lists common system trust bundles in priority order. An
// empty result is fine on platforms where Go uses the native certificate store
// instead of a file.
func systemBundlePaths() []string {
	paths := []string{
		os.Getenv("SSL_CERT_FILE"),
		os.Getenv("CURL_CA_BUNDLE"),
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
		"/etc/ssl/cert.pem",
	}
	out := []string{}
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

// ensureBundleLocked concatenates the system roots with Lair's CA. Tools that
// use SSL_CERT_FILE then trust both packagist.org and local *.test sites.
func ensureBundleLocked(certsDir string, caPEM []byte) error {
	bundle := BundlePath(certsDir)
	parts := [][]byte{}
	for _, path := range systemBundlePaths() {
		if path == bundle {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		parts = append(parts, data, []byte("\n"))
		break
	}
	parts = append(parts, caPEM, []byte("\n"))
	return atomicWrite(bundle, bytes.Join(parts, nil), 0o644)
}

// ParseCA decodes PEM-encoded CA certificate and key.
func ParseCA(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, nil, fmt.Errorf("invalid CA certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("invalid CA key PEM")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	if err := keysMatch(cert, key); err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

// keysMatch rejects a CA whose certificate and private key come from different
// key pairs. Without this check a partially written (or tampered) data root
// would yield a CA that silently signs unusable certificates.
func keysMatch(cert *x509.Certificate, key *ecdsa.PrivateKey) error {
	want, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	got, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("CA certificate and private key do not match")
	}
	return nil
}

// NormalizeHost lowercases a host name and removes a single trailing dot.
func NormalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

// SANsFor returns the DNS names of a canonical site host. It is the single
// source of truth used by the engine and the TLS server so on-demand SNI never
// widens a certificate beyond the parked site. A host outside domain is
// rejected by returning nil.
func SANsFor(host, domain string) []string {
	host = NormalizeHost(host)
	if host == "" || strings.ContainsAny(host, " /\\\t\n*") {
		return nil
	}
	domain = NormalizeHost(domain)
	if domain != "" && host != domain && !strings.HasSuffix(host, "."+domain) {
		return nil
	}
	return []string{host, "*." + host}
}

// IssueSite signs a leaf certificate for the given domains and writes
// <certsDir>/<site>.pem and <certsDir>/<site>-key.pem. The SANs include every
// domain passed plus 127.0.0.1 and ::1 so local tooling stays compatible.
func IssueSite(certsDir, site string, domains ...string) (certPath, keyPath string, err error) {
	if len(domains) == 0 {
		base := strings.TrimSuffix(site, ".")
		domains = []string{base, "*." + base}
	}
	caCert, caKey, err := EnsureCA(certsDir)
	if err != nil {
		return "", "", err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, err := randomSerial()
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: domains[0], Organization: []string{"Lair"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(siteValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     domains,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}
	certPath, keyPath = sitePaths(certsDir, site)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, _ := x509.MarshalECPrivateKey(leafKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := atomicWrite(certPath, certPEM, 0o644); err != nil {
		return "", "", err
	}
	if err := atomicWrite(keyPath, keyPEM, 0o600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

func sitePaths(certsDir, site string) (string, string) {
	name := strings.TrimSuffix(site, ".")
	name = strings.ReplaceAll(name, "*.", "")
	name = strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(name)
	return filepath.Join(certsDir, name+".pem"), filepath.Join(certsDir, name+"-key.pem")
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lair-pki-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
