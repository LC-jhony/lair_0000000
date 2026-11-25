// Package assets embeds the artefacts that must travel inside the single
// binary: the manifest-signing public key and the two logo variants.
package assets

import (
	"crypto/ed25519"
	_ "embed"
	"encoding/hex"
	"fmt"
)

// OverridePublicKeyHex is a build-time-only replacement for the embedded
// development key. It is injected with -ldflags -X from the production public
// key; there is deliberately no environment override, because a runtime
// override would let any local process replace the trust root.
var OverridePublicKeyHex string

//go:embed logo-dark.svg
var LogoDark []byte

//go:embed logo-light.svg
var LogoLight []byte

//go:embed public.ed25519
var publicKey []byte

//go:embed catalog.json
var exampleCatalog []byte

//go:embed catalog.json.sig
var exampleCatalogSig []byte

// ExampleCatalog returns the signed example catalog shipped inside the binary.
// It is a development fixture, not a production feed: it points at official
// vendor archives with their published SHA-256 sums and lets a fresh install
// work before the remote catalog has ever been fetched.
func ExampleCatalog() (data, sig []byte) {
	return append([]byte(nil), exampleCatalog...), append([]byte(nil), exampleCatalogSig...)
}

// PublicKey returns the embedded Ed25519 public key used to verify manifests.
// The private key is deliberately NOT embedded: a leaked private key in a
// shipped binary would let anyone sign malicious manifests.
func PublicKey() (ed25519.PublicKey, error) {
	if OverridePublicKeyHex != "" {
		decoded, err := hex.DecodeString(OverridePublicKeyHex)
		if err != nil {
			return nil, fmt.Errorf("invalid build-time public key: %w", err)
		}
		if len(decoded) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("build-time public key has %d bytes, want %d", len(decoded), ed25519.PublicKeySize)
		}
		return ed25519.PublicKey(decoded), nil
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("embedded public key has %d bytes, want %d", len(publicKey), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(append([]byte(nil), publicKey...)), nil
}
