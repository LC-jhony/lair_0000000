package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusIncludesDNSAndCA(t *testing.T) {
	e, _ := testEnv(t)

	dns := e.statusDNS("test")
	if dns.Host != "lair.test" {
		t.Fatalf("DNS host = %q", dns.Host)
	}

	ca := e.statusCA()
	if ca.Path == "" {
		t.Fatal("CA path is empty")
	}

	status, err := e.Status()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"dns"`, `"ca"`, `"system_trusted"`, `"browser_trusted"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("status JSON missing %s: %s", key, data)
		}
	}
}
