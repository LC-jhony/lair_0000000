// Command catalog-sign signs a Lair catalog manifest with a raw Ed25519
// private key (64 bytes). The key is development-only and is never embedded.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
)

func main() {
	in := flag.String("in", "catalog/manifest.json", "manifest to sign")
	out := flag.String("out", "catalog/manifest.json.sig", "signature output")
	key := flag.String("key", "keys/private.ed25519", "raw 64-byte Ed25519 private key")
	flag.Parse()

	data, err := os.ReadFile(*in)
	if err != nil {
		fail(err)
	}
	priv, err := os.ReadFile(*key)
	if err != nil {
		fail(err)
	}
	if len(priv) != ed25519.PrivateKeySize {
		fail(fmt.Errorf("private key must be %d bytes, got %d", ed25519.PrivateKeySize, len(priv)))
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(priv), data))
	if err := os.WriteFile(*out, []byte(sig+"\n"), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("signed %s -> %s\n", *in, *out)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "catalog-sign:", err)
	os.Exit(1)
}
