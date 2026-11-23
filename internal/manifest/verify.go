package manifest

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"path"
	"strings"
	"time"
)

// VerifyOptions controls manifest verification policy.
type VerifyOptions struct {
	Channel    string    // required channel; empty accepts whatever the manifest says
	Now        time.Time // clock used for expiry checks; zero means time.Now
	LastSerial int64     // highest serial already accepted; 0 disables the check
	Schema     int       // required schema; 0 means app.SchemaVersion is used
}

// Verify checks the Ed25519 signature and every policy rule, in this order:
// signature, JSON, schema, channel, expiry, anti-rollback. Rejecting early on
// the signature means an attacker cannot even make us parse their payload.
func Verify(data, sigBase64 []byte, pub ed25519.PublicKey, opts VerifyOptions) (*Manifest, error) {
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid embedded public key size")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigBase64)))
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("invalid signature size %d", len(sig))
	}
	if !ed25519.Verify(pub, data, sig) {
		return nil, fmt.Errorf("manifest signature is invalid")
	}

	m, err := Parse(data)
	if err != nil {
		return nil, err
	}

	schema := opts.Schema
	if schema == 0 {
		schema = 1
	}
	if m.Schema != schema {
		return nil, fmt.Errorf("unknown schema %d (expected %d)", m.Schema, schema)
	}
	if opts.Channel != "" && m.Channel != opts.Channel {
		return nil, fmt.Errorf("channel mismatch: got %q, want %q", m.Channel, opts.Channel)
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	if m.Expires.IsZero() {
		return nil, fmt.Errorf("manifest has no expiry")
	}
	if !now.Before(m.Expires) {
		return nil, fmt.Errorf("manifest expired at %s", m.Expires.Format(time.RFC3339))
	}
	if opts.LastSerial > 0 && m.Serial < opts.LastSerial {
		return nil, fmt.Errorf("manifest serial %d is older than accepted serial %d (rollback)", m.Serial, opts.LastSerial)
	}
	return m, nil
}

// VerifyArtifact checks that an artifact and every component are complete and
// not yanked before we spend bandwidth downloading them.
func VerifyArtifact(a Artifact) error {
	if err := verifySource(a.URL, a.SHA256); err != nil {
		return fmt.Errorf("primary artifact: %w", err)
	}
	if reason, yanked := a.IsYanked(); yanked {
		return fmt.Errorf("build %s was yanked: %s", a.Build, reason)
	}
	if a.Raw != "" {
		if err := safeRelative(a.Raw); err != nil {
			return fmt.Errorf("invalid raw destination %q: %w", a.Raw, err)
		}
	}
	if err := verifyFiles(a.Files); err != nil {
		return fmt.Errorf("primary artifact files: %w", err)
	}
	for i, c := range a.Components {
		if err := verifySource(c.URL, c.SHA256); err != nil {
			return fmt.Errorf("component %d: %w", i, err)
		}
		if err := verifyFiles(c.Files); err != nil {
			return fmt.Errorf("component %d files: %w", i, err)
		}
	}
	return nil
}

func verifySource(url, sha256 string) error {
	if url == "" {
		return fmt.Errorf("no url")
	}
	if len(sha256) != 64 {
		return fmt.Errorf("sha256 must be 64 hex chars, got %d", len(sha256))
	}
	for _, r := range sha256 {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return fmt.Errorf("sha256 is not hexadecimal")
		}
	}
	return nil
}

func verifyFiles(files []FileMap) error {
	for _, f := range files {
		if err := safeRelative(f.From); err != nil {
			return fmt.Errorf("invalid source %q: %w", f.From, err)
		}
		if err := safeRelative(f.To); err != nil {
			return fmt.Errorf("invalid destination %q: %w", f.To, err)
		}
	}
	return nil
}

// safeRelative rejects absolute paths and parent traversal in archive member
// names and install destinations.
func safeRelative(name string) error {
	if name == "" {
		return fmt.Errorf("empty path")
	}
	n := strings.ReplaceAll(name, `\`, "/")
	if strings.HasPrefix(n, "/") || path.IsAbs(n) {
		return fmt.Errorf("absolute path")
	}
	cleaned := path.Clean(n)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path escapes install directory")
	}
	return nil
}
