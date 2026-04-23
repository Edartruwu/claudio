package tools

import (
	"testing"

	"github.com/Abraxas-365/claudio/internal/tasks"
)

// bashToolFromRegistry extracts the *BashTool from a registry, failing if absent.
func bashToolFromRegistry(t *testing.T, r *Registry) *BashTool {
	t.Helper()
	tool, err := r.Get("Bash")
	if err != nil {
		t.Fatalf("registry has no Bash tool: %v", err)
	}
	bt, ok := tool.(*BashTool)
	if !ok {
		t.Fatalf("Bash tool is not *BashTool, got %T", tool)
	}
	return bt
}

func TestRegistry_Clone_BashToolIsDistinctInstance(t *testing.T) {
	orig := NewRegistry()
	orig.Register(&BashTool{})

	clone := orig.Clone()

	origBT := bashToolFromRegistry(t, orig)
	cloneBT := bashToolFromRegistry(t, clone)

	if origBT == cloneBT {
		t.Error("Clone produced same *BashTool pointer — want distinct instance")
	}
}

func TestRegistry_Clone_BashToolRuntimeIndependent(t *testing.T) {
	dir := t.TempDir()

	orig := NewRegistry()
	orig.Register(&BashTool{})

	clone := orig.Clone()

	rt1 := tasks.NewRuntime(dir)
	rt2 := tasks.NewRuntime(dir)

	// Set different runtimes on each registry.
	orig.SetTaskRuntime(rt1)
	clone.SetTaskRuntime(rt2)

	origBT := bashToolFromRegistry(t, orig)
	cloneBT := bashToolFromRegistry(t, clone)

	if origBT.TaskRuntime != rt1 {
		t.Error("original BashTool should have rt1")
	}
	if cloneBT.TaskRuntime != rt2 {
		t.Error("cloned BashTool should have rt2")
	}
	if origBT.TaskRuntime == cloneBT.TaskRuntime {
		t.Error("original and clone share same TaskRuntime pointer — want independent")
	}
}

func TestRegistry_SetTaskRuntime_SetsBashToolRuntime(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry()
	r.Register(&BashTool{})

	rt := tasks.NewRuntime(dir)
	r.SetTaskRuntime(rt)

	bt := bashToolFromRegistry(t, r)
	if bt.TaskRuntime != rt {
		t.Errorf("BashTool.TaskRuntime = %p, want %p", bt.TaskRuntime, rt)
	}
}

func TestRegistry_SetTaskRuntime_NoopWhenNoBashTool(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry()
	// No BashTool registered — SetTaskRuntime must not panic.
	rt := tasks.NewRuntime(dir)
	r.SetTaskRuntime(rt)
}
