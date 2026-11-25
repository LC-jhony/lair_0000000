package engine

import (
	"strings"
	"testing"
)

func TestSQLFingerprintGroupsEquivalentQueries(t *testing.T) {
	a := sqlFingerprint("SELECT * FROM users WHERE id = 42")
	b := sqlFingerprint("select * from users where id = 99 /* trace */")
	if a != b {
		t.Fatalf("fingerprints differ: %q vs %q", a, b)
	}
	in := sqlFingerprint("select * from tags where id in (1, 2, 3)")
	if !strings.Contains(in, "in (?)") {
		t.Fatalf("IN list not collapsed: %q", in)
	}
}

func TestDumpInsightsDetectSlowAndNPlusOne(t *testing.T) {
	events := []DumpEvent{
		{ID: "1", Site: "acme.test", RequestID: "r1", Type: "queries",
			Message: "select * from users where id = 1", DurationMS: 150},
		{ID: "2", Site: "acme.test", RequestID: "r1", Type: "queries",
			Message: "select * from users where id = 2", DurationMS: 120},
		{ID: "3", Site: "acme.test", RequestID: "r1", Type: "queries",
			Message: "select * from users where id = 3", DurationMS: 110},
		{ID: "4", Site: "acme.test", RequestID: "r1", Type: "queries",
			Message: "select * from users where id = 4", DurationMS: 105},
		{ID: "5", Site: "acme.test", RequestID: "r1", Type: "queries",
			Message: "select * from users where id = 5", DurationMS: 101},
		{ID: "6", Site: "acme.test", RequestID: "r1", Type: "views", Message: "rendered"},
	}
	ins := buildDumpInsights(events, 0, 0) // defaults: 100 ms, 5 repeats
	if ins.TotalQueries != 5 {
		t.Fatalf("total queries = %d, want 5", ins.TotalQueries)
	}
	if len(ins.SlowQueries) != 5 {
		t.Fatalf("slow queries = %d, want 5", len(ins.SlowQueries))
	}
	if len(ins.NPlusOne) != 1 || ins.NPlusOne[0].Count != 5 {
		t.Fatalf("N+1 = %+v, want one bucket of 5", ins.NPlusOne)
	}
	if len(ins.Queries) != 1 {
		t.Fatalf("query shapes = %d, want 1", len(ins.Queries))
	}
}

func TestRequestTimelineGroupsByID(t *testing.T) {
	events := []DumpEvent{
		{ID: "1", RequestID: "r2", Site: "acme.test", Time: "2026-11-25T10:00:02Z",
			Type: "queries", DurationMS: 120},
		{ID: "2", RequestID: "r1", Site: "acme.test", Time: "2026-11-25T10:00:00Z",
			Type: "views"},
		{ID: "3", RequestID: "r2", Site: "acme.test", Time: "2026-11-25T10:00:01Z",
			Type: "logs"},
	}
	tl := buildRequestTimeline(events)
	if len(tl) != 2 {
		t.Fatalf("timeline groups = %d, want 2", len(tl))
	}
	// Newest group first.
	if tl[0].RequestID != "r2" || tl[0].Count != 2 || tl[0].DurationMS != 120 {
		t.Fatalf("newest group = %+v", tl[0])
	}
	if tl[0].Events[0].ID != "3" {
		t.Fatalf("events not sorted within group: %+v", tl[0].Events)
	}
}
