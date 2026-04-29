package windows

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestLiveBuffer_InitialState(t *testing.T) {
	lb := NewLiveBuffer("test")
	if lb.Done() {
		t.Fatal("new buffer should not be done")
	}
	if lb.Status() != "running" {
		t.Fatalf("initial status: want running, got %q", lb.Status())
	}
	if lines := lb.Lines(); len(lines) != 0 {
		t.Fatalf("initial lines: want 0, got %d", len(lines))
	}
}

func TestLiveBuffer_AppendAndLines(t *testing.T) {
	lb := NewLiveBuffer("test")
	lb.Append("line one")
	lb.Append("line two")

	lines := lb.Lines()
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if lines[0] != "line one" {
		t.Fatalf("lines[0]: want %q, got %q", "line one", lines[0])
	}
	if lines[1] != "line two" {
		t.Fatalf("lines[1]: want %q, got %q", "line two", lines[1])
	}
}

func TestLiveBuffer_LinesSnapshotIsCopy(t *testing.T) {
	lb := NewLiveBuffer("test")
	lb.Append("original")
	snap := lb.Lines()
	snap[0] = "mutated"

	// Internal state must not be affected.
	fresh := lb.Lines()
	if fresh[0] != "original" {
		t.Fatalf("snapshot mutated internal state: got %q", fresh[0])
	}
}

func TestLiveBuffer_SetDone_Status(t *testing.T) {
	lb := NewLiveBuffer("test")
	lb.SetDone("done")

	if !lb.Done() {
		t.Fatal("want Done()=true after SetDone")
	}
	if lb.Status() != "done" {
		t.Fatalf("status: want done, got %q", lb.Status())
	}
}

func TestLiveBuffer_SetDone_Error(t *testing.T) {
	lb := NewLiveBuffer("test")
	lb.SetDone("error")

	if lb.Status() != "error" {
		t.Fatalf("status: want error, got %q", lb.Status())
	}
	if !lb.Done() {
		t.Fatal("want Done()=true after SetDone(error)")
	}
}

func TestLiveBuffer_SetDone_InvalidDefaultsToDone(t *testing.T) {
	lb := NewLiveBuffer("test")
	lb.SetDone("bogus")

	if lb.Status() != "done" {
		t.Fatalf("invalid status should default to done, got %q", lb.Status())
	}
}

func TestLiveBuffer_AppendThreadSafety(t *testing.T) {
	lb := NewLiveBuffer("test")
	const goroutines = 50
	const linesEach = 100

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < linesEach; j++ {
				lb.Append(fmt.Sprintf("g%d-l%d", id, j))
			}
		}(i)
	}
	wg.Wait()

	lines := lb.Lines()
	if len(lines) != goroutines*linesEach {
		t.Fatalf("thread-safety: want %d lines, got %d", goroutines*linesEach, len(lines))
	}
}

func TestLiveBuffer_Buffer_ReturnsValidBuffer(t *testing.T) {
	lb := NewLiveBuffer("mybuf")
	lb.Append("hello")
	lb.Append("world")

	buf := lb.Buffer()
	if buf == nil {
		t.Fatal("Buffer() returned nil")
	}
	if buf.Name != "mybuf" {
		t.Fatalf("buffer name: want mybuf, got %q", buf.Name)
	}
	if buf.Render == nil {
		t.Fatal("buffer Render func is nil")
	}
}

func TestLiveBuffer_Buffer_RenderShowsLines(t *testing.T) {
	lb := NewLiveBuffer("test")
	lb.Append("alpha")
	lb.Append("beta")

	buf := lb.Buffer()
	out := buf.Render(80, 24)
	if !strings.Contains(out, "alpha") {
		t.Fatalf("render missing alpha: %q", out)
	}
	if !strings.Contains(out, "beta") {
		t.Fatalf("render missing beta: %q", out)
	}
}

func TestLiveBuffer_Buffer_RenderTailsOnHeightLimit(t *testing.T) {
	lb := NewLiveBuffer("test")
	for i := 0; i < 10; i++ {
		lb.Append(fmt.Sprintf("line%d", i))
	}

	buf := lb.Buffer()
	// height=3 → only last 3 lines visible
	out := buf.Render(80, 3)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("height=3 should show 3 lines, got %d: %q", len(lines), out)
	}
	if lines[2] != "line9" {
		t.Fatalf("last line: want line9, got %q", lines[2])
	}
}

func TestLiveBuffer_Buffer_RenderTrimsWidth(t *testing.T) {
	lb := NewLiveBuffer("test")
	lb.Append("abcdefghij") // 10 chars

	buf := lb.Buffer()
	out := buf.Render(5, 24)
	// Rendered line trimmed to 5 runes
	if out != "abcde" {
		t.Fatalf("width trim: want abcde, got %q", out)
	}
}
