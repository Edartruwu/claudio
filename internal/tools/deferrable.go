package tools

// deferrable is an embeddable struct that implements DeferrableTool.
// Embed this in any tool that should be deferred to save tokens.
type deferrable struct {
	hint string
}

func (d deferrable) ShouldDefer() bool   { return true }
func (d deferrable) SearchHint() string  { return d.hint }

// newDeferrable creates a deferrable with the given search hint.
func newDeferrable(hint string) deferrable {
	return deferrable{hint: hint}
}
