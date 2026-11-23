package desktopapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeCaller struct{}

func (fakeCaller) Call(_ context.Context, action string, payload, out any) error {
	if action == "boom" {
		return context.DeadlineExceeded
	}
	b, _ := json.Marshal(map[string]any{"action": action, "ok": true})
	return json.Unmarshal(b, out)
}

type contextCaller struct {
	ctx context.Context
}

func (c *contextCaller) Call(ctx context.Context, _ string, _ any, _ any) error {
	c.ctx = ctx
	return nil
}

func TestHandlerUsesRequestDetachedContext(t *testing.T) {
	caller := &contextCaller{}
	s, err := New(caller, "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a request whose context is cancelled while the handler runs.
	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(`{"action":"status"}`)).WithContext(reqCtx)
	req.Header.Set("X-Lair-Token", "secret-token")
	cancel()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if caller.ctx == nil {
		t.Fatal("dispatcher received no context")
	}
	if caller.ctx.Err() != nil {
		t.Fatalf("job context was cancelled with the request: %v", caller.ctx.Err())
	}
}

func TestHandlerUsesDaemonLifetimeContext(t *testing.T) {
	caller := &contextCaller{}
	s, err := New(caller, "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	lifetime, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.mu.Lock()
	s.lifetime = lifetime
	s.mu.Unlock()

	post(s, "secret-token", "", `{"action":"status"}`)
	if caller.ctx != lifetime {
		t.Fatal("dispatcher did not receive the daemon lifetime context")
	}
	cancel()
	if caller.ctx.Err() == nil {
		t.Fatal("daemon cancellation must reach running jobs")
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(fakeCaller{}, "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func post(s *Server, token, origin, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(body))
	if token != "" {
		req.Header.Set("X-Lair-Token", token)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func TestNoStaticInterface(t *testing.T) {
	s := newTestServer(t)
	for _, path := range []string{"/", "/index.html", "/app.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 (no web UI)", path, w.Code)
		}
	}
}

func TestAuth(t *testing.T) {
	s := newTestServer(t)
	body := `{"action":"status"}`
	if w := post(s, "", "", body); w.Code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", w.Code)
	}
	if w := post(s, "wrong", "", body); w.Code != http.StatusUnauthorized {
		t.Errorf("bad token = %d, want 401", w.Code)
	}
	// Tokens in the URL are no longer accepted.
	req := httptest.NewRequest(http.MethodPost, "/api?token=secret-token", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("query token = %d, want 401", w.Code)
	}
	if w := post(s, "secret-token", "", body); w.Code != http.StatusOK {
		t.Errorf("valid call = %d: %s", w.Code, w.Body.String())
	}
}

func TestCall(t *testing.T) {
	s := newTestServer(t)
	w := post(s, "secret-token", "", `{"action":"status","payload":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("valid call = %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Action string `json:"action"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Data.Action != "status" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestErrors(t *testing.T) {
	s := newTestServer(t)
	if w := post(s, "secret-token", "", `not json`); w.Code != http.StatusBadRequest {
		t.Errorf("bad json = %d, want 400", w.Code)
	}
	if w := post(s, "secret-token", "", `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("missing action = %d, want 400", w.Code)
	}
	if w := post(s, "secret-token", "", `{"action":"boom"}`); w.Code != http.StatusInternalServerError {
		t.Errorf("caller error = %d, want 500", w.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api = %d, want 405", w.Code)
	}
}

func TestCORS(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	for _, origin := range []string{"null", "http://localhost:5173", "http://127.0.0.1:5173"} {
		req := httptest.NewRequest(http.MethodOptions, "/api", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Errorf("preflight %s = %d, want 204", origin, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("preflight %s ACAO = %q", origin, got)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(`{"action":"status"}`))
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("X-Lair-Token", "secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("foreign origin = %d, want 403", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("foreign origin got ACAO %q", got)
	}
}
