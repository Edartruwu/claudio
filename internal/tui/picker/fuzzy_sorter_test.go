package picker

import (
	"math"
	"testing"
)

func TestFuzzySorter_EmptyPrompt(t *testing.T) {
	s := NewFuzzySorter()
	e := Entry{Ordinal: "anything"}
	got := s.Score("", e)
	if got != 0.0 {
		t.Fatalf("empty prompt: want 0.0, got %v", got)
	}
}

func TestFuzzySorter_ExactMatch(t *testing.T) {
	s := NewFuzzySorter()
	e := Entry{Ordinal: "hello"}
	got := s.Score("hello", e)
	if got != 0.0 {
		t.Fatalf("exact match: want 0.0, got %v", got)
	}
}

func TestFuzzySorter_PrefixMatch(t *testing.T) {
	s := NewFuzzySorter()
	e := Entry{Ordinal: "hello"}
	got := s.Score("hel", e)
	// prefix at idx=0 → 0.0/5 = 0.0
	if got != 0.0 {
		t.Fatalf("prefix match: want 0.0, got %v", got)
	}
}

func TestFuzzySorter_SubstringMidMatch(t *testing.T) {
	s := NewFuzzySorter()
	// "ell" at idx=1 in "hello" (len=5) → 1/5 = 0.2
	e := Entry{Ordinal: "hello"}
	got := s.Score("ell", e)
	want := float64(1) / float64(5)
	if got != want {
		t.Fatalf("mid match: want %v, got %v", want, got)
	}
}

func TestFuzzySorter_NoMatch(t *testing.T) {
	s := NewFuzzySorter()
	e := Entry{Ordinal: "hello"}
	got := s.Score("xyz", e)
	if got != math.MaxFloat64 {
		t.Fatalf("no match: want MaxFloat64, got %v", got)
	}
}

func TestFuzzySorter_CaseInsensitive(t *testing.T) {
	s := NewFuzzySorter()
	e := Entry{Ordinal: "Hello"}
	got := s.Score("hello", e)
	// case-insensitive prefix → 0.0
	if got != 0.0 {
		t.Fatalf("case-insensitive: want 0.0, got %v", got)
	}
}

func TestFuzzySorter_EmptyOrdinal(t *testing.T) {
	s := NewFuzzySorter()
	e := Entry{Ordinal: ""}
	got := s.Score("any", e)
	if got != math.MaxFloat64 {
		t.Fatalf("empty ordinal: want MaxFloat64, got %v", got)
	}
}

func TestFuzzySorter_EarlierMatchScoresLower(t *testing.T) {
	// Verify ordering: earlier match = lower score = better rank.
	s := NewFuzzySorter()
	early := Entry{Ordinal: "abcde"} // "b" at idx=1 → 1/5
	late := Entry{Ordinal: "xyzba"}  // "b" at idx=3 → 3/5
	scoreEarly := s.Score("b", early)
	scoreLate := s.Score("b", late)
	if scoreEarly >= scoreLate {
		t.Fatalf("early match should score lower: early=%v late=%v", scoreEarly, scoreLate)
	}
}
