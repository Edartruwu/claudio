package capabilities_test

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/capabilities"
	"github.com/Abraxas-365/claudio/internal/tools"
)

func TestRegistry_IsKnown(t *testing.T) {
	r := capabilities.New()

	if r.IsKnown("design") {
		t.Fatal("empty registry: IsKnown should return false")
	}

	r.Register("design")
	if !r.IsKnown("design") {
		t.Fatal("after Register: IsKnown should return true")
	}
	if r.IsKnown("other") {
		t.Fatal("unregistered cap should not be known")
	}
}

func TestRegistry_Register_MultipleFactories(t *testing.T) {
	r := capabilities.New()

	calls := 0
	factory := func(reg *tools.Registry, deps capabilities.ToolDeps) { calls++ }

	r.Register("test-cap", factory, factory, factory)

	reg := tools.NewRegistry()
	applied := r.ApplyToRegistry([]string{"test-cap"}, reg, nil, nil, "", nil)

	if !applied {
		t.Fatal("ApplyToRegistry should return true when cap matched")
	}
	if calls != 3 {
		t.Fatalf("expected 3 factory calls, got %d", calls)
	}
}

func TestRegistry_Register_Append(t *testing.T) {
	r := capabilities.New()

	calls := 0
	factory := func(reg *tools.Registry, deps capabilities.ToolDeps) { calls++ }

	r.Register("cap", factory)
	r.Register("cap", factory) // append to same cap

	reg := tools.NewRegistry()
	r.ApplyToRegistry([]string{"cap"}, reg, nil, nil, "", nil)

	if calls != 2 {
		t.Fatalf("appended factories: expected 2 calls, got %d", calls)
	}
}

func TestRegistry_ApplyToRegistry_UnknownCap(t *testing.T) {
	r := capabilities.New()
	r.Register("design")

	reg := tools.NewRegistry()
	applied := r.ApplyToRegistry([]string{"unknown"}, reg, nil, nil, "", nil)

	if applied {
		t.Fatal("unknown cap: ApplyToRegistry should return false")
	}
}

func TestRegistry_ApplyToRegistry_EmptyCaps(t *testing.T) {
	r := capabilities.New()
	r.Register("design")

	reg := tools.NewRegistry()
	applied := r.ApplyToRegistry([]string{}, reg, nil, nil, "", nil)

	if applied {
		t.Fatal("empty caps: ApplyToRegistry should return false")
	}
}

func TestRegistry_ApplyToRegistry_MultipleCapsSomeKnown(t *testing.T) {
	r := capabilities.New()

	called := map[string]int{}
	r.Register("a", func(reg *tools.Registry, deps capabilities.ToolDeps) { called["a"]++ })
	r.Register("b", func(reg *tools.Registry, deps capabilities.ToolDeps) { called["b"]++ })

	reg := tools.NewRegistry()
	applied := r.ApplyToRegistry([]string{"a", "unknown", "b"}, reg, nil, nil, "", nil)

	if !applied {
		t.Fatal("some known caps: ApplyToRegistry should return true")
	}
	if called["a"] != 1 || called["b"] != 1 {
		t.Fatalf("expected a=1 b=1 calls, got %v", called)
	}
}

func TestRegistry_ConcurrentRegisterIsKnown(t *testing.T) {
	r := capabilities.New()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			r.Register("cap")
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_ = r.IsKnown("cap")
	}
	<-done
}
