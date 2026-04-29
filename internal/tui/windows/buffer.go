// Package windows provides Buffer/Window/Manager primitives for the TUI
// window system. Buffers are named content providers; Windows are viewports
// into Buffers with layout hints; Manager is the registry and z-ordered stack.
package windows

// Buffer is a named content provider. Render is called by its owning Window
// to produce the visible string for a given viewport size. State holds
// arbitrary plugin-owned data.
type Buffer struct {
	Name   string
	Render func(width, height int) string
	State  any
}
