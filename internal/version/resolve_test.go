package version

import (
	"reflect"
	"testing"
)

func TestParseAndCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"8.4.3", "8.4.2", 1},
		{"8.4.0", "8.4", 0},
		{"22.11.0", "23.0.0", -1},
		{"v8.5.1", "8.5.1", 0},
		{"8.4.3+N2", "8.4.3+N1", 1},
	}
	for _, c := range cases {
		av, err := Parse(c.a)
		if err != nil {
			t.Fatalf("parse %s: %v", c.a, err)
		}
		bv, err := Parse(c.b)
		if err != nil {
			t.Fatalf("parse %s: %v", c.b, err)
		}
		if got := av.Compare(bv); got != c.want {
			t.Errorf("compare(%s,%s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if _, err := Parse("garbage"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLatestInMajor(t *testing.T) {
	available := []string{"22.4.0", "22.11.0", "22.9.1", "20.19.0"}
	got, err := LatestInMajor("22.11.0", available)
	if err != nil {
		t.Fatal(err)
	}
	if got != "22.11.0" {
		t.Fatalf("got %s", got)
	}
	got, err = LatestInMajor("22.5.0", available)
	if err != nil {
		t.Fatal(err)
	}
	if got != "22.11.0" {
		t.Fatalf("major resolution got %s, want 22.11.0", got)
	}
	if _, err := LatestInMajor("24.0.0", available); err == nil {
		t.Fatal("expected error for missing major")
	}
}

func TestExactOrLatest(t *testing.T) {
	available := []string{"22.4.0", "22.11.0", "20.19.0"}
	if got, _ := ExactOrLatest("22.4.0", available); got != "22.4.0" {
		t.Fatalf("exact got %s", got)
	}
	if got, _ := ExactOrLatest("22.9.0", available); got != "22.11.0" {
		t.Fatalf("fallback got %s", got)
	}
	if got, err := ExactOrLatest("", available); err != nil || got != "22.11.0" {
		t.Fatalf("empty got %s, %v", got, err)
	}
	if got, err := ExactOrLatest("latest", available); err != nil || got != "22.11.0" {
		t.Fatalf("latest got %s, %v", got, err)
	}
	if got, err := ExactOrLatest("22", available); err != nil || got != "22.11.0" {
		t.Fatalf("major got %s, %v", got, err)
	}
	if _, err := ExactOrLatest("", nil); err == nil {
		t.Fatal("empty catalog accepted")
	}
}

func TestExactOrLatestMinorLine(t *testing.T) {
	available := []string{"11.8.9", "11.4.13", "10.11.19"}
	if got, err := ExactOrLatest("11.4", available); err != nil || got != "11.4.13" {
		t.Fatalf("minor line got %s, %v", got, err)
	}
	if got, _ := ExactOrLatest("11", available); got != "11.8.9" {
		t.Fatalf("major got %s", got)
	}
	if _, err := ExactOrLatest("11.3", available); err == nil {
		t.Fatal("missing minor line accepted")
	}
}

func TestSortNewest(t *testing.T) {
	v := []string{"8.1.2", "8.5.0", "8.4.9", "8.4.10"}
	SortNewest(v)
	want := []string{"8.5.0", "8.4.10", "8.4.9", "8.1.2"}
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("got %v, want %v", v, want)
	}
}

func TestMinorLine(t *testing.T) {
	if !IsMinorLine("8.4") {
		t.Fatal("8.4 should be a minor line")
	}
	if IsMinorLine("8.4.3") || IsMinorLine("lts/*") {
		t.Fatal("unexpected minor line")
	}
	v, _ := Parse("8.4.3")
	if v.MinorLine() != "8.4" || v.MajorLine() != "8" {
		t.Fatalf("lines: %s %s", v.MinorLine(), v.MajorLine())
	}
}
