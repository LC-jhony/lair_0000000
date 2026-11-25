package assets

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestPublicKeyUsesBuildTimeOverride(t *testing.T) {
	old := OverridePublicKeyHex
	t.Cleanup(func() { OverridePublicKeyHex = old })

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	OverridePublicKeyHex = hex.EncodeToString(pub)
	got, err := PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(pub) {
		t.Fatal("build-time override was not used")
	}

	OverridePublicKeyHex = "deadbeef"
	if _, err := PublicKey(); err == nil {
		t.Fatal("short override accepted")
	}
}
