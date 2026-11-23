package node

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/manifest"
)

func TestNormalizeOfficialVersion(t *testing.T) {
	cases := map[string]string{
		"22.16.0":    "v22.16.0",
		" v22.16.0 ": "v22.16.0",
		"v24.13.1":   "v24.13.1",
	}
	for in, want := range cases {
		got, err := NormalizeOfficialVersion(in)
		if err != nil || got != want {
			t.Errorf("normalize %q = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "22", "lts", "v22.16", "../22.16.0", "22.16.0/evil"} {
		if _, err := NormalizeOfficialVersion(in); err == nil {
			t.Errorf("accepted invalid version %q", in)
		}
	}
}

func TestOfficialArtifact(t *testing.T) {
	const sha = "fb870226119d47378fa9c92c4535389c72dae14fcc7b47e6fdcc82c43de5a547"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v22.16.0/SHASUMS256.txt" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("aaa  node-v22.16.0-linux-arm64.tar.gz\n" + sha + "  node-v22.16.0-linux-x64.tar.gz\n"))
	}))
	defer server.Close()

	old := OfficialBaseURL
	OfficialBaseURL = server.URL + "/"
	t.Cleanup(func() { OfficialBaseURL = old })

	art, err := OfficialArtifact(context.Background(), server.Client(), "22.16.0", "linux-x64")
	if err != nil {
		t.Fatal(err)
	}
	if art.SHA256 != sha {
		t.Fatalf("sha = %s", art.SHA256)
	}
	if !strings.HasSuffix(art.URL, "/v22.16.0/node-v22.16.0-linux-x64.tar.gz") {
		t.Fatalf("url = %s", art.URL)
	}
	if err := manifest.VerifyArtifact(art); err != nil {
		t.Fatalf("artifact rejected: %v", err)
	}
	if _, err := OfficialArtifact(context.Background(), server.Client(), "22.16.0", "plan9-x64"); err == nil {
		t.Fatal("unsupported platform accepted")
	}
}
