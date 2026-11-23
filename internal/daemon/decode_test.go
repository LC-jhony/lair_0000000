package daemon

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestDecodeRejectsMalformedPayload(t *testing.T) {
	var req JobRequest
	if err := decode(nil, &req); err != nil {
		t.Fatalf("empty payload should be allowed: %v", err)
	}
	if err := decode(json.RawMessage(`{"id":"job-1"}`), &req); err != nil || req.ID != "job-1" {
		t.Fatalf("valid payload = %+v, %v", req, err)
	}
	if err := decode(json.RawMessage(`{oops`), &req); err == nil {
		t.Fatal("malformed JSON accepted")
	}
	if err := decode(json.RawMessage(`42`), &req); err == nil {
		t.Fatal("wrong JSON type accepted")
	}
}

func TestDecodeSiteEnabledDistinguishesFalseFromAbsent(t *testing.T) {
	var req SiteRequest
	if err := decode(json.RawMessage(`{"op":"set-enabled","name":"blog.test","enabled":false}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.Op != "set-enabled" || req.Name != "blog.test" || req.Enabled == nil || *req.Enabled {
		t.Fatalf("enabled=false payload = %+v", req)
	}
	var plain SiteRequest
	if err := decode(json.RawMessage(`{"op":"config","name":"blog.test"}`), &plain); err != nil {
		t.Fatal(err)
	}
	if plain.Enabled != nil {
		t.Fatalf("absent enabled field became %v", *plain.Enabled)
	}
}

func TestHandlersPropagateDecodeErrors(t *testing.T) {
	d, _ := testDispatcher(t)
	// Every payload-taking handler goes through decode(); a malformed payload
	// must be reported instead of silently acting on a zero-value request.
	for _, action := range []string{
		ActionVersions, ActionInstallPHP, ActionInstallNode, ActionInstallService,
		ActionJob, ActionJobCancel, ActionLogs, ActionPHPUse, ActionNodeDefault,
		ActionServiceUse, ActionServiceRestart, ActionServiceStop,
		ActionServiceUninstall, ActionCreateDB, ActionDBDefault, ActionPark,
		ActionLink, ActionUnlink, ActionSecure, ActionWeb, ActionProject,
		ActionPin, ActionIsolate, ActionBootstrap, ActionEnsureShims,
		ActionUninstall, ActionMail, ActionDumps, ActionDebugger, ActionSite,
	} {
		fn, ok := d.handlers[action]
		if !ok {
			t.Fatalf("handler %s not registered", action)
		}
		if _, err := fn(context.Background(), json.RawMessage(`{oops`)); err == nil {
			t.Errorf("%s accepted a malformed payload", action)
		}
	}
}

func TestServerEnvelopeErrorAndRequestLimit(t *testing.T) {
	d, _ := testDispatcher(t)
	srv := NewServer("unused.sock", "secret-token")
	Register(srv, d)
	ctx := context.Background()

	// A payload that is valid JSON but not a JobRequest must reach decode()
	// and produce an "invalid payload" error on the wire.
	client, server := net.Pipe()
	go srv.handleConn(ctx, server)
	if err := json.NewEncoder(client).Encode(map[string]any{
		"token": "secret-token", "action": ActionJob, "payload": 42,
	}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	if err := json.NewDecoder(client).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.OK || !strings.Contains(resp.Error, "payload") {
		t.Fatalf("response = %+v", resp)
	}
	client.Close()

	// Oversized requests are cut off by the server's read limit.
	client, server = net.Pipe()
	go srv.handleConn(ctx, server)
	payload := strings.Repeat("x", maxRequestBytes+1024)
	go func() {
		_ = json.NewEncoder(client).Encode(map[string]any{
			"token": "secret-token", "action": ActionJob, "payload": payload,
		})
	}()
	if err := json.NewDecoder(client).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.OK || !strings.Contains(resp.Error, "invalid request") {
		t.Fatalf("oversized request response = %+v", resp)
	}
	client.Close()
}
