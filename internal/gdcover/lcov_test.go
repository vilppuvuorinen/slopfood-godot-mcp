package gdcover

import (
	"math"
	"strings"
	"testing"
)

func TestParseHitsCountsExecutions(t *testing.T) {
	hits, err := ParseHits(strings.NewReader("3\n5\n3\n"))
	if err != nil {
		t.Fatalf("ParseHits: %v", err)
	}
	if hits[3] != 2 || hits[5] != 1 || len(hits) != 2 {
		t.Fatalf("hits = %v, want {3:2, 5:1}", hits)
	}
}

func TestParseHitsRejectsGarbage(t *testing.T) {
	if _, err := ParseHits(strings.NewReader("3\nnot-a-line\n")); err == nil {
		t.Fatal("ParseHits: want error, got nil")
	}
}

func TestCoverWriteLCOVAndPercent(t *testing.T) {
	cover := Cover{Source: "res://x.gd", Lines: []int{1, 3, 5}, Hits: map[int]int{3: 2, 5: 1}}

	var out strings.Builder
	if err := cover.WriteLCOV(&out); err != nil {
		t.Fatalf("WriteLCOV: %v", err)
	}
	for _, want := range []string{
		"SF:res://x.gd", "DA:1,0", "DA:3,2", "DA:5,1", "LF:3", "LH:2", "end_of_record",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report missing %q:\n%s", want, out.String())
		}
	}
	if got, want := cover.HitLines(), 2; got != want {
		t.Errorf("HitLines = %d, want %d", got, want)
	}
	if got := cover.Percent(); math.Abs(got-200.0/3) > 1e-9 {
		t.Errorf("Percent = %v, want %v", got, 200.0/3)
	}
}

func TestCoverEmptyScriptIsFullyCovered(t *testing.T) {
	cover := Cover{Source: "res://empty.gd"}
	if got := cover.Percent(); got != 100 {
		t.Errorf("Percent = %v, want 100", got)
	}
}
