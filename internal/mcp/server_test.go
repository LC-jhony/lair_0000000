package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/lair-sh/lair/internal/daemon"
)

// callRecord captures one forwarded daemon action.
type callRecord struct {
	action  string
	payload any
}

// fakeCaller mirrors daemon.Dispatcher.Call: it marshals a canned result into
// out and records the action/payload it received.
type fakeCaller struct {
	responses map[string]any
	err       error
	calls     []callRecord
}

func (f *fakeCaller) Call(_ context.Context, action string, payload, out any) error {
	f.calls = append(f.calls, callRecord{action: action, payload: payload})
	if f.err != nil {
		return f.err
	}
	data, ok := f.responses[action]
	if !ok {
		data = map[string]any{"ok": true}
	}
	if out == nil {
		return nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// runServe feeds newline-delimited requests through Serve and returns the
// decoded reply objects.
func runServe(t *testing.T, c daemon.Caller, lines ...string) []map[string]any {
	t.Helper()
	input := strings.Join(lines, "\n")
	if input != "" {
		input += "\n"
	}
	var out bytes.Buffer
	if err := New(c).Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var replies []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("invalid reply line %q: %v", line, err)
		}
		replies = append(replies, msg)
	}
	return replies
}

func field(t *testing.T, m map[string]any, key string) any {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("reply missing %q: %v", key, m)
	}
	return v
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T (%v)", v, v)
	}
	return m
}

func TestServe(t *testing.T) {
	const statusJSON = `{"node":"22","php":"8.3"}`

	tests := []struct {
		name        string
		input       string
		wantID      any
		wantCode    float64
		wantMessage string
		wantNoReply bool
		checkResult func(t *testing.T, result any)
	}{
		{
			name:  "initialize",
			input: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
			wantID: float64(1),
			checkResult: func(t *testing.T, result any) {
				r := asMap(t, result)
				if got := field(t, r, "protocolVersion"); got != protocolVersion {
					t.Errorf("protocolVersion = %v, want %s", got, protocolVersion)
				}
				info := asMap(t, field(t, r, "serverInfo"))
				if got := field(t, info, "name"); got != "lair" {
					t.Errorf("serverInfo.name = %v, want lair", got)
				}
				if field(t, info, "version") == nil {
					t.Error("serverInfo.version missing")
				}
				caps := asMap(t, field(t, r, "capabilities"))
				if _, ok := caps["tools"]; !ok {
					t.Error("capabilities.tools missing")
				}
			},
		},
		{
			name:  "ping",
			input: `{"jsonrpc":"2.0","id":"ping-1","method":"ping"}`,
			wantID: "ping-1",
			checkResult: func(t *testing.T, result any) {
				if got := asMap(t, result); len(got) != 0 {
					t.Errorf("ping result = %v, want empty object", got)
				}
			},
		},
		{
			name:  "tools/list",
			input: `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
			wantID: float64(2),
			checkResult: func(t *testing.T, result any) {
				raw, _ := json.Marshal(field(t, asMap(t, result), "tools"))
				var listed []struct {
					Name        string         `json:"name"`
					Description string         `json:"description"`
					InputSchema map[string]any `json:"inputSchema"`
				}
				if err := json.Unmarshal(raw, &listed); err != nil {
					t.Fatalf("decode tools: %v", err)
				}
				want := []string{"lair_status", "lair_versions", "lair_jobs", "lair_logs", "lair_services_health", "lair_site_data"}
				var got []string
				for _, tool := range listed {
					got = append(got, tool.Name)
					if tool.Description == "" {
						t.Errorf("tool %s has no description", tool.Name)
					}
					if tool.InputSchema["type"] != "object" {
						t.Errorf("tool %s schema type = %v", tool.Name, tool.InputSchema["type"])
					}
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("tool names = %v, want %v", got, want)
				}
			},
		},
		{
			name:  "successful tools/call",
			input: `{"jsonrpc":"2.0","id":"call-1","method":"tools/call","params":{"name":"lair_status","arguments":{}}}`,
			wantID: "call-1",
			checkResult: func(t *testing.T, result any) {
				content := asMap(t, result)["content"].([]any)
				if len(content) != 1 {
					t.Fatalf("content length = %d, want 1", len(content))
				}
				block := asMap(t, content[0])
				if got := field(t, block, "type"); got != "text" {
					t.Errorf("content type = %v, want text", got)
				}
				if got := field(t, block, "text"); got != statusJSON {
					t.Errorf("content text = %v, want %s", got, statusJSON)
				}
			},
		},
		{
			name:        "unknown tool",
			input:       `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"lair_nuke","arguments":{}}}`,
			wantID:      float64(3),
			wantCode:    errInvalidParams,
			wantMessage: `unknown tool "lair_nuke"`,
		},
		{
			name:        "unknown method",
			input:       `{"jsonrpc":"2.0","id":4,"method":"does/not/exist"}`,
			wantID:      float64(4),
			wantCode:    errMethodNotFound,
			wantMessage: "method not found: does/not/exist",
		},
		{
			name:        "malformed JSON",
			input:       `{"jsonrpc":"2.0","id":5,"method":`,
			wantID:      nil,
			wantCode:    errParseError,
			wantMessage: "parse error",
		},
		{
			name:        "invalid request version",
			input:       `{"jsonrpc":"1.0","id":6,"method":"ping"}`,
			wantID:      float64(6),
			wantCode:    errInvalidRequest,
			wantMessage: "jsonrpc must be",
		},
		{
			name:        "missing tool argument",
			input:       `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"lair_versions","arguments":{}}}`,
			wantID:      float64(7),
			wantCode:    errInvalidParams,
			wantMessage: `missing required argument "package"`,
		},
		{
			name:        "invalid tools/call params",
			input:       `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":"not-an-object"}`,
			wantID:      float64(8),
			wantCode:    errInvalidParams,
			wantMessage: "invalid tools/call params",
		},
		{
			name:        "notifications/initialized gets no reply",
			input:       `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			wantNoReply: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &fakeCaller{responses: map[string]any{
				daemon.ActionStatus: map[string]any{"php": "8.3", "node": "22"},
			}}
			replies := runServe(t, caller, tc.input)
			if tc.wantNoReply {
				if len(replies) != 0 {
					t.Fatalf("got %d replies, want none: %v", len(replies), replies)
				}
				return
			}
			if len(replies) != 1 {
				t.Fatalf("got %d replies, want 1: %v", len(replies), replies)
			}
			reply := replies[0]
			if got := field(t, reply, "jsonrpc"); got != "2.0" {
				t.Errorf("jsonrpc = %v", got)
			}
			if tc.wantID != nil {
				if got := reply["id"]; !reflect.DeepEqual(got, tc.wantID) {
					t.Errorf("id = %v (%T), want %v", got, got, tc.wantID)
				}
			}
			if tc.wantCode != 0 {
				errObj := asMap(t, field(t, reply, "error"))
				if got := errObj["code"]; got != tc.wantCode {
					t.Errorf("error.code = %v, want %v", got, tc.wantCode)
				}
				if msg, _ := errObj["message"].(string); !strings.Contains(msg, tc.wantMessage) {
					t.Errorf("error.message = %q, want substring %q", msg, tc.wantMessage)
				}
				if _, ok := reply["result"]; ok {
					t.Error("error reply must not carry a result")
				}
				return
			}
			if _, ok := reply["error"]; ok {
				t.Fatalf("unexpected error: %v", reply["error"])
			}
			if tc.checkResult != nil {
				tc.checkResult(t, field(t, reply, "result"))
			}
		})
	}
}

func TestToolsCallActions(t *testing.T) {
	tests := []struct {
		name        string
		tool        string
		arguments   string
		wantAction  string
		wantPayload any
	}{
		{"status", "lair_status", "", daemon.ActionStatus, nil},
		{"versions", "lair_versions", `{"package":"php"}`, daemon.ActionVersions, daemon.VersionRequest{Package: "php"}},
		{"jobs", "lair_jobs", "", daemon.ActionJobs, nil},
		{"logs", "lair_logs", `{"name":"php.log"}`, daemon.ActionLogs, daemon.LogsRequest{Name: "php.log"}},
		{"services health", "lair_services_health", "", daemon.ActionServicesHealth, nil},
		{"site data", "lair_site_data", `{"site":"blog.test"}`, daemon.ActionSiteData, daemon.SiteDataRequest{Name: "blog.test"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.arguments
			if args == "" {
				args = "{}"
			}
			input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, tc.tool, args)
			caller := &fakeCaller{responses: map[string]any{tc.wantAction: map[string]any{"action": tc.wantAction}}}
			replies := runServe(t, caller, input)
			if len(replies) != 1 {
				t.Fatalf("got %d replies, want 1", len(replies))
			}
			if _, ok := replies[0]["error"]; ok {
				t.Fatalf("unexpected error: %v", replies[0]["error"])
			}
			if len(caller.calls) != 1 {
				t.Fatalf("got %d daemon calls, want 1", len(caller.calls))
			}
			got := caller.calls[0]
			if got.action != tc.wantAction {
				t.Errorf("action = %q, want %q", got.action, tc.wantAction)
			}
			gotPayload, _ := json.Marshal(got.payload)
			wantPayload, _ := json.Marshal(tc.wantPayload)
			if !bytes.Equal(gotPayload, wantPayload) {
				t.Errorf("payload = %s, want %s", gotPayload, wantPayload)
			}
			block := asMap(t, asMap(t, replies[0]["result"])["content"].([]any)[0])
			wantText := fmt.Sprintf(`{"action":%q}`, tc.wantAction)
			if got := block["text"]; got != wantText {
				t.Errorf("text = %v, want %s", got, wantText)
			}
		})
	}
}

func TestToolErrorBecomesJSONRPCError(t *testing.T) {
	caller := &fakeCaller{err: fmt.Errorf("daemon offline")}
	input := `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"lair_logs","arguments":{"name":"php.log"}}}`
	replies := runServe(t, caller, input)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
	errObj := asMap(t, field(t, replies[0], "error"))
	if got := errObj["code"]; got != float64(errInternal) {
		t.Errorf("error.code = %v, want %v", got, errInternal)
	}
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "daemon offline") {
		t.Errorf("error.message = %q, want daemon error text", msg)
	}
	if field(t, replies[0], "id") != float64(9) {
		t.Errorf("id was not echoed: %v", replies[0]["id"])
	}
}

func TestMultipleRequestsAndBlankLines(t *testing.T) {
	caller := &fakeCaller{}
	replies := runServe(t, caller,
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		"",
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	)
	if len(replies) != 2 {
		t.Fatalf("got %d replies, want 2: %v", len(replies), replies)
	}
	if field(t, replies[0], "id") != float64(1) || field(t, replies[1], "id") != float64(2) {
		t.Errorf("reply ids = %v", []any{replies[0]["id"], replies[1]["id"]})
	}
}

func TestCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	err := New(&fakeCaller{}).Serve(ctx, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`), &out)
	if err != context.Canceled {
		t.Errorf("Serve error = %v, want context.Canceled", err)
	}
}
