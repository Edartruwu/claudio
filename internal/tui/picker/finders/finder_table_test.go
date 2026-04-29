package finders

import (
	"context"
	"testing"

	"github.com/Abraxas-365/claudio/internal/tui/picker"
)

func TestTableFinder_StreamsAllEntries(t *testing.T) {
	entries := []picker.Entry{
		{Display: "A", Ordinal: "a"},
		{Display: "B", Ordinal: "b"},
		{Display: "C", Ordinal: "c"},
	}
	f := NewTableFinder(entries)
	defer f.Close()

	ch := f.Find(context.Background())
	var got []picker.Entry
	for e := range ch {
		got = append(got, e)
	}

	if len(got) != len(entries) {
		t.Fatalf("want %d entries, got %d", len(entries), len(got))
	}
}

func TestTableFinder_CorrectOrdinal(t *testing.T) {
	entries := []picker.Entry{
		{Display: "First", Ordinal: "first"},
		{Display: "Second", Ordinal: "second"},
	}
	f := NewTableFinder(entries)
	defer f.Close()

	ch := f.Find(context.Background())
	var got []picker.Entry
	for e := range ch {
		got = append(got, e)
	}

	for i, e := range entries {
		if got[i].Ordinal != e.Ordinal {
			t.Fatalf("entry[%d] ordinal: want %q, got %q", i, e.Ordinal, got[i].Ordinal)
		}
	}
}

func TestTableFinder_CorrectDisplay(t *testing.T) {
	entries := []picker.Entry{
		{Display: "hello world", Ordinal: "hello"},
	}
	f := NewTableFinder(entries)
	defer f.Close()

	ch := f.Find(context.Background())
	e := <-ch
	if e.Display != "hello world" {
		t.Fatalf("display: want %q, got %q", "hello world", e.Display)
	}
}

func TestTableFinder_EmptyEntries(t *testing.T) {
	f := NewTableFinder(nil)
	defer f.Close()

	ch := f.Find(context.Background())
	count := 0
	for range ch {
		count++
	}
	if count != 0 {
		t.Fatalf("empty table: want 0 entries, got %d", count)
	}
}

func TestTableFinder_CancelContext(t *testing.T) {
	// Large table; cancel before draining → no deadlock.
	entries := make([]picker.Entry, 1000)
	for i := range entries {
		entries[i] = picker.Entry{Ordinal: "x"}
	}
	f := NewTableFinder(entries)
	defer f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch := f.Find(ctx)
	// Read one, then cancel.
	<-ch
	cancel()
	// Drain to completion to ensure goroutine exits.
	for range ch {
	}
}
