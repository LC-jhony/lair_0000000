// Package desktopapi exposes Lair's local HTTP bridge to the native desktop
// app. There is no web interface: every call goes through the same dispatcher
// and token as the CLI, so the daemon remains the single source of truth.
package desktopapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Caller mirrors daemon.Caller without importing daemon (the daemon starts
// this server, so importing it would create an import cycle).
type Caller interface {
	Call(ctx context.Context, action string, payload, out any) error
}

// Server serves the authenticated POST /api bridge on loopback.
type Server struct {
	caller Caller
	token  string
	server *http.Server

	mu       sync.Mutex
	lifetime context.Context // daemon context; job actions must not use r.Context()
}

type apiRequest struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type apiResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// New builds the desktop API server. token must be non-empty: every call is
// authenticated with it using a constant-time comparison.
func New(caller Caller, token string) (*Server, error) {
	if caller == nil {
		return nil, fmt.Errorf("desktopapi: nil caller")
	}
	if token == "" {
		return nil, fmt.Errorf("desktopapi: empty token")
	}
	return &Server{caller: caller, token: token}, nil
}

// Handler returns the root http.Handler (API only, no static files).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", s.handleAPI)
	return mux
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if !s.allowOrigin(w, r) {
		writeJSON(w, http.StatusForbidden, apiResponse{Error: "origin not allowed"})
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Error: "method not allowed"})
		return
	}
	// The token travels in a header only, never in the URL (no token leaks in
	// logs or browser history).
	token := r.Header.Get("X-Lair-Token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
		writeJSON(w, http.StatusUnauthorized, apiResponse{Error: "invalid token"})
		return
	}

	var req apiRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.Action == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Error: "missing action"})
		return
	}

	var out any
	// r.Context() is cancelled as soon as this handler returns. Actions that
	// start background jobs must outlive the HTTP request, so use the daemon's
	// lifetime context when available and a request-detached context as a
	// fallback (for example when Handler is used directly in tests).
	s.mu.Lock()
	callCtx := s.lifetime
	s.mu.Unlock()
	if callCtx == nil {
		callCtx = context.WithoutCancel(r.Context())
	}
	if err := s.caller.Call(callCtx, req.Action, req.Payload, &out); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: out})
}

// allowOrigin only permits the native app (file:// => "null") and local dev
// servers. A random website must not be able to reach the loopback bridge,
// even if it somehow learned the token.
func (s *Server) allowOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser clients (curl, the Electron main process) send no Origin.
		return true
	}
	if !allowedOrigin(origin) {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Lair-Token")
	w.Header().Set("Access-Control-Max-Age", "600")
	w.Header().Add("Vary", "Origin")
	return true
}

func allowedOrigin(origin string) bool {
	// "null" is sent by sandboxed iframes and data: URLs and is trivial to
	// forge. The Electron app now performs calls from the main process, so it
	// sends no Origin at all; browsers must present a real loopback origin.
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ListenAndServe serves on addr until ctx is cancelled, then shuts down
// gracefully. The caller supplies a loopback address.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	s.mu.Lock()
	s.lifetime = ctx
	s.mu.Unlock()
	s.server = &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		err := s.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
