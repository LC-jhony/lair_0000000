package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSurfaceHandlers(t *testing.T) {
	d, paths := testDispatcher(t)
	ctx := context.Background()

	var mail MailResult
	if err := d.Call(ctx, ActionMail, MailRequest{Op: "status"}, &mail); err != nil {
		t.Fatal(err)
	}
	if mail.Port != 2525 || !strings.Contains(mail.Env, "MAIL_SCHEME=null") {
		t.Fatalf("mail status = %+v", mail)
	}

	var dumps DumpsResult
	if err := d.Call(ctx, ActionDumps, DumpsRequest{Op: "options", Types: []string{"queries"}}, &dumps); err != nil {
		t.Fatal(err)
	}
	if !dumps.Options.Queries || dumps.Options.Logs {
		t.Fatalf("dumps options = %+v", dumps.Options)
	}
	if err := d.Call(ctx, ActionDumps, DumpsRequest{
		Op: "ingest", Site: "acme.test", Type: "queries", Message: "select 1",
	}, &dumps); err != nil {
		t.Fatal(err)
	}
	if dumps.Count != 1 {
		t.Fatalf("dumps count = %d", dumps.Count)
	}
	if err := d.Call(ctx, ActionDumps, DumpsRequest{Op: "list"}, &dumps); err != nil {
		t.Fatal(err)
	}
	if len(dumps.Events) != 1 || dumps.Events[0].Message != "select 1" {
		t.Fatalf("dumps events = %+v", dumps.Events)
	}

	var debugger DebuggerResult
	if err := d.Call(ctx, ActionDebugger, DebuggerRequest{Op: "detect-on"}, &debugger); err != nil {
		t.Fatal(err)
	}
	if !debugger.Detect || debugger.Breakpoints || debugger.Port != 9003 {
		t.Fatalf("debugger = %+v", debugger)
	}
	if err := d.Call(ctx, ActionDebugger, DebuggerRequest{Op: "breakpoints-on"}, &debugger); err != nil {
		t.Fatal(err)
	}
	if !debugger.Breakpoints {
		t.Fatalf("breakpoints = %+v", debugger)
	}

	var list NameList
	if err := os.WriteFile(filepath.Join(paths.Log, "lair.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.Call(ctx, ActionLogsList, nil, &list); err != nil {
		t.Fatal(err)
	}
	if list.Items == nil {
		t.Fatal("logs items missing")
	}
	if list.Sources == nil {
		t.Fatal("logs sources missing")
	}
}

func TestSurfaceActionsRejectBadWireTypes(t *testing.T) {
	d, _ := testDispatcher(t)
	for _, action := range []string{ActionMail, ActionDumps, ActionDebugger} {
		fn, ok := d.handlers[action]
		if !ok {
			t.Fatalf("handler %s not registered", action)
		}
		// JSON arrays are valid JSON but not valid request objects.
		if _, err := fn(context.Background(), json.RawMessage(`[]`)); err == nil {
			t.Errorf("%s accepted a JSON array payload", action)
		}
	}
}
