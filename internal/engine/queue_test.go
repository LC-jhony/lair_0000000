package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueueControlValidatesSiteAndDriver(t *testing.T) {
	e, _ := testEnv(t)
	unknown := e.QueueControl(context.Background(), "no-existe.test")
	if !strings.Contains(unknown.Error, "sitio desconocido") {
		t.Fatalf("unknown site = %+v", unknown)
	}

	dir := linkTestSite(t, e, "blog.test")
	writeTestFile(t, filepath.Join(dir, ".env"), "QUEUE_CONNECTION=sqlsrv\n")
	q := e.QueueControl(context.Background(), "blog.test")
	if q.Driver != "sqlsrv" || !strings.Contains(q.Error, "no soportado") {
		t.Fatalf("unsupported driver = %+v", q)
	}
	if q.Hint == "" || q.Worker == "" {
		t.Fatalf("missing worker hint: %+v", q)
	}
}
