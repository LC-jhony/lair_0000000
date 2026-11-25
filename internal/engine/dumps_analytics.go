package engine

import (
	"regexp"
	"sort"
	"strings"
)

// Dump analytics turn the raw event feed into the two signals a developer
// actually looks for: a query that is slow, and the same query repeated many
// times inside one request (the classic N+1 pattern). A third view groups all
// events of a request into a timeline. All are derived; nothing is persisted
// separately, so old dumps keep working.

const (
	defaultSlowMS  = 100.0
	defaultMinRepl = 5
	maxInsightRows = 20
)

// QueryStat aggregates repeated query shapes.
type QueryStat struct {
	Fingerprint string  `json:"fingerprint"`
	Count       int     `json:"count"`
	TotalMS     float64 `json:"total_ms"`
	MaxMS       float64 `json:"max_ms"`
}

// SlowQuery is one query above the duration threshold.
type SlowQuery struct {
	ID         string  `json:"id"`
	Site       string  `json:"site"`
	RequestID  string  `json:"request_id,omitempty"`
	Request    string  `json:"request,omitempty"`
	SQL        string  `json:"sql"`
	Time       string  `json:"time"`
	DurationMS float64 `json:"duration_ms"`
}

// NPlusOne is a query shape repeated within one request.
type NPlusOne struct {
	Fingerprint string  `json:"fingerprint"`
	Site        string  `json:"site"`
	RequestID   string  `json:"request_id,omitempty"`
	Request     string  `json:"request,omitempty"`
	SampleSQL   string  `json:"sample_sql"`
	Count       int     `json:"count"`
	TotalMS     float64 `json:"total_ms"`
}

// DumpInsights holds the computed query signals for the current event page.
type DumpInsights struct {
	SlowQueries  []SlowQuery `json:"slow_queries"`
	NPlusOne     []NPlusOne  `json:"n_plus_one"`
	Queries      []QueryStat `json:"queries"`
	TotalQueries int         `json:"total_queries"`
}

// RequestTimeline groups all captured events belonging to one request.
type RequestTimeline struct {
	RequestID  string      `json:"request_id"`
	Site       string      `json:"site"`
	Request    string      `json:"request,omitempty"`
	Start      string      `json:"start"`
	End        string      `json:"end"`
	Count      int         `json:"count"`
	DurationMS float64     `json:"duration_ms"`
	Events     []DumpEvent `json:"events"`
}

var (
	sqlCommentRE = regexp.MustCompile(`/\*.*?\*/|--[^\n]*`)
	sqlStringRE  = regexp.MustCompile(`'(?:''|[^'])*'|"(?:[^"])*"`)
	sqlNumberRE  = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
	sqlSpaceRE   = regexp.MustCompile(`\s+`)
	sqlInRE      = regexp.MustCompile(`\bin\s*\(\s*\?(?:\s*,\s*\?)*\s*\)`)
)

// sqlFingerprint removes literals so equivalent ORM-generated queries share
// one bucket. It is deliberately conservative: unknown SQL stays as written.
func sqlFingerprint(sql string) string {
	s := strings.ToLower(strings.TrimSpace(sql))
	s = sqlCommentRE.ReplaceAllString(s, " ")
	s = sqlStringRE.ReplaceAllString(s, "?")
	s = sqlNumberRE.ReplaceAllString(s, "?")
	s = sqlInRE.ReplaceAllString(s, "in (?)")
	return strings.TrimSpace(sqlSpaceRE.ReplaceAllString(s, " "))
}

func requestKey(ev DumpEvent) string {
	if ev.RequestID != "" {
		return ev.RequestID
	}
	// Older events have no id; the URI is the best available correlation key.
	return ev.Site + "|" + ev.Request
}

func buildDumpInsights(events []DumpEvent, slowMS float64, minRepeats int) DumpInsights {
	if slowMS <= 0 {
		slowMS = defaultSlowMS
	}
	if minRepeats <= 0 {
		minRepeats = defaultMinRepl
	}
	out := DumpInsights{}
	type group struct {
		stat    QueryStat
		sample  string
		site    string
		reqID   string
		request string
	}
	groups := map[string]*group{}
	for _, ev := range events {
		if !strings.EqualFold(ev.Type, "queries") {
			continue
		}
		out.TotalQueries++
		fp := sqlFingerprint(ev.Message)
		g := groups[requestKey(ev)+"|"+fp]
		if g == nil {
			g = &group{stat: QueryStat{Fingerprint: fp}, sample: ev.Message,
				site: ev.Site, reqID: ev.RequestID, request: ev.Request}
			groups[requestKey(ev)+"|"+fp] = g
		}
		g.stat.Count++
		g.stat.TotalMS += ev.DurationMS
		if ev.DurationMS > g.stat.MaxMS {
			g.stat.MaxMS = ev.DurationMS
		}
		if ev.DurationMS >= slowMS {
			out.SlowQueries = append(out.SlowQueries, SlowQuery{
				ID: ev.ID, Site: ev.Site, RequestID: ev.RequestID, Request: ev.Request,
				SQL: ev.Message, Time: ev.Time, DurationMS: ev.DurationMS,
			})
		}
	}
	for _, g := range groups {
		out.Queries = append(out.Queries, g.stat)
		if g.stat.Count >= minRepeats {
			out.NPlusOne = append(out.NPlusOne, NPlusOne{
				Fingerprint: g.stat.Fingerprint, Site: g.site,
				RequestID: g.reqID, Request: g.request, SampleSQL: g.sample,
				Count: g.stat.Count, TotalMS: g.stat.TotalMS,
			})
		}
	}
	sort.Slice(out.SlowQueries, func(i, j int) bool {
		return out.SlowQueries[i].DurationMS > out.SlowQueries[j].DurationMS
	})
	sort.Slice(out.NPlusOne, func(i, j int) bool {
		return out.NPlusOne[i].Count > out.NPlusOne[j].Count
	})
	sort.Slice(out.Queries, func(i, j int) bool {
		return out.Queries[i].Count > out.Queries[j].Count
	})
	if len(out.SlowQueries) > maxInsightRows {
		out.SlowQueries = out.SlowQueries[:maxInsightRows]
	}
	if len(out.NPlusOne) > maxInsightRows {
		out.NPlusOne = out.NPlusOne[:maxInsightRows]
	}
	if len(out.Queries) > maxInsightRows {
		out.Queries = out.Queries[:maxInsightRows]
	}
	return out
}

func buildRequestTimeline(events []DumpEvent) []RequestTimeline {
	order := map[string]int{}
	byKey := map[string]*RequestTimeline{}
	for _, ev := range events {
		key := requestKey(ev)
		t := byKey[key]
		if t == nil {
			t = &RequestTimeline{RequestID: ev.RequestID, Site: ev.Site, Request: ev.Request}
			byKey[key] = t
			order[key] = len(order)
		}
		t.Events = append(t.Events, ev)
		t.Count++
		if t.Start == "" || ev.Time < t.Start {
			t.Start = ev.Time
		}
		if ev.Time > t.End {
			t.End = ev.Time
		}
		if strings.EqualFold(ev.Type, "queries") {
			t.DurationMS += ev.DurationMS
		}
	}
	out := make([]RequestTimeline, 0, len(byKey))
	for _, t := range byKey {
		sort.Slice(t.Events, func(i, j int) bool { return t.Events[i].Time < t.Events[j].Time })
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].End > out[j].End })
	if len(out) > maxInsightRows {
		out = out[:maxInsightRows]
	}
	return out
}
