// Package mcp implements a minimal, self-contained Model Context Protocol
// server over newline-delimited JSON-RPC 2.0. stdin is the request channel and
// stdout is reserved exclusively for protocol replies: diagnostics go to a log
// logger on stderr so they can never corrupt the framing.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/lair-sh/lair/internal/daemon"
	buildversion "github.com/lair-sh/lair/internal/version"
)

// protocolVersion is the MCP revision this server speaks.
const protocolVersion = "2024-11-05"

// Standard JSON-RPC 2.0 error codes.
const (
	errParseError     = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternal       = -32603
)

// Server forwards MCP tool calls to a daemon.Caller. A caller may be the
// in-process dispatcher or the remote socket client; both share the same
// action names and payloads.
type Server struct {
	caller daemon.Caller
	log    *log.Logger
}

// New builds a server. All logging is written to stderr.
func New(caller daemon.Caller) *Server {
	return &Server{
		caller: caller,
		log:    log.New(os.Stderr, "lair-mcp: ", log.LstdFlags),
	}
}

// rpcRequest is one JSON-RPC request line. ID and params stay as raw messages
// so an arbitrary JSON id (string or number) is echoed back verbatim.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// rpcError is the JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcResponse is one reply line. Exactly one of result/error is populated.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// tool describes one MCP tool and its JSON Schema input contract.
type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// toolContent is a single text content block of a tools/call result.
type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// toolResult is the result object returned by tools/call.
type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// tools is the registry exposed to MCP clients. Tools are read-only views of
// local daemon state: no tool mutates projects, services or configuration.
var tools = []tool{
	{
		Name:        "lair_status",
		Description: "Report the local Lair status: PHP, Node, services, sites and running install jobs.",
		InputSchema: objectSchema(),
	},
	{
		Name:        "lair_versions",
		Description: "List the versions available to install for a package (for example php, node or a service id).",
		InputSchema: objectSchema("package"),
	},
	{
		Name:        "lair_jobs",
		Description: "List Lair install jobs and their current status.",
		InputSchema: objectSchema(),
	},
	{
		Name:        "lair_logs",
		Description: "Read the contents of a named Lair log file.",
		InputSchema: objectSchema("name"),
	},
	{
		Name:        "lair_services_health",
		Description: "Report the health of the local services managed by Lair.",
		InputSchema: objectSchema(),
	},
	{
		Name:        "lair_site_data",
		Description: "Read the data (metadata, environment, runtime versions) of a parked Lair site.",
		InputSchema: objectSchema("site"),
	},
}

// objectSchema builds a JSON Schema for an object with the given required
// string arguments. With no arguments it describes an empty object.
func objectSchema(args ...string) map[string]any {
	properties := map[string]any{}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(args) > 0 {
		required := make([]string, 0, len(args))
		for _, name := range args {
			properties[name] = map[string]any{
				"type":        "string",
				"description": argumentDescription(name),
			}
			required = append(required, name)
		}
		schema["required"] = required
	}
	return schema
}

func argumentDescription(name string) string {
	switch name {
	case "package":
		return "Package whose available versions should be listed."
	case "name":
		return "Name of the log file to read."
	case "site":
		return "Host name of the parked site to inspect."
	default:
		return "String argument."
	}
}

// Serve reads JSON-RPC requests line by line until EOF or ctx cancellation.
// It writes exactly one reply per request; notifications (no id) get none.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	// Tool arguments and log payloads can be large; allow multi-MB lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	enc := json.NewEncoder(w)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		resp := s.handle(ctx, line)
		if resp == nil {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("write MCP response: %w", err)
		}
	}
}

// handle parses and dispatches one request. It returns nil for notifications.
func (s *Server) handle(ctx context.Context, line []byte) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(json.RawMessage("null"), errParseError, "parse error: "+err.Error())
	}

	notification := len(req.ID) == 0 || bytes.Equal(req.ID, []byte("null"))
	if req.JSONRPC != "2.0" {
		if notification {
			return nil
		}
		return errorResponse(req.ID, errInvalidRequest, "invalid request: jsonrpc must be \"2.0\"")
	}

	var result any
	var rpcErr *rpcError
	switch req.Method {
	case "initialize":
		result = initializeResult()
	case "notifications/initialized":
		return nil
	case "ping":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": tools}
	case "tools/call":
		result, rpcErr = s.callTool(ctx, req.Params)
	default:
		rpcErr = &rpcError{Code: errMethodNotFound, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}

	if notification {
		return nil
	}
	if rpcErr != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
	}
	return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

// initializeResult is the MCP initialize handshake reply.
func initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"serverInfo": map[string]any{
			"name":    "lair",
			"version": buildversion.BuildVersion,
		},
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
	}
}

// callParams is the params object of a tools/call request.
type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// callTool maps a tool name to a daemon action, calls it and wraps the result
// as MCP text content. Errors become JSON-RPC errors with a readable message.
func (s *Server) callTool(ctx context.Context, rawParams json.RawMessage) (any, *rpcError) {
	var params callParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, &rpcError{Code: errInvalidParams, Message: "invalid tools/call params: " + err.Error()}
	}

	var args map[string]json.RawMessage
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return nil, &rpcError{Code: errInvalidParams, Message: "invalid tool arguments: " + err.Error()}
		}
	}

	var action string
	var payload any
	switch params.Name {
	case "lair_status":
		action = daemon.ActionStatus
	case "lair_versions":
		pkg, err := stringArg(args, "package")
		if err != nil {
			return nil, invalidArgError(err)
		}
		action, payload = daemon.ActionVersions, daemon.VersionRequest{Package: pkg}
	case "lair_jobs":
		action = daemon.ActionJobs
	case "lair_logs":
		name, err := stringArg(args, "name")
		if err != nil {
			return nil, invalidArgError(err)
		}
		action, payload = daemon.ActionLogs, daemon.LogsRequest{Name: name}
	case "lair_services_health":
		action = daemon.ActionServicesHealth
	case "lair_site_data":
		site, err := stringArg(args, "site")
		if err != nil {
			return nil, invalidArgError(err)
		}
		action, payload = daemon.ActionSiteData, daemon.SiteDataRequest{Name: site}
	default:
		return nil, &rpcError{Code: errInvalidParams, Message: fmt.Sprintf("unknown tool %q", params.Name)}
	}

	// Each caller marshals the daemon result into out. json.RawMessage keeps
	// the bytes verbatim so the MCP client receives the exact daemon payload.
	var raw json.RawMessage
	if err := s.caller.Call(ctx, action, payload, &raw); err != nil {
		s.log.Printf("tool %s (action %s) failed: %v", params.Name, action, err)
		return nil, &rpcError{Code: errInternal, Message: fmt.Sprintf("%s: %v", params.Name, err)}
	}
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	return toolResult{Content: []toolContent{{Type: "text", Text: string(raw)}}}, nil
}

// stringArg reads a required string argument from a tool call.
func stringArg(args map[string]json.RawMessage, key string) (string, error) {
	value, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required argument %q", key)
	}
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		return "", fmt.Errorf("argument %q must be a string", key)
	}
	return s, nil
}

func invalidArgError(err error) *rpcError {
	return &rpcError{Code: errInvalidParams, Message: err.Error()}
}

func errorResponse(id json.RawMessage, code int, message string) *rpcResponse {
	return &rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	}
}
