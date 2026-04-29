// Package lua — PanelRegistry and supporting types for the claudio.win.new_panel API.
package lua

import (
	"log"
	"sync"

	glua "github.com/yuin/gopher-lua"
)

// SectionDef describes one section inside a panel.
type SectionDef struct {
	ID        string
	Title     string
	Weight    int
	MinHeight int
	plugin    *loadedPlugin
	renderFn  *glua.LFunction
}

// CallRender invokes the Lua render function with (w, h) and returns the string.
func (s *SectionDef) CallRender(w, h int) string {
	if s.plugin == nil || s.plugin.L == nil || s.renderFn == nil {
		return ""
	}
	s.plugin.mu.Lock()
	defer s.plugin.mu.Unlock()
	defer func() {
		if rv := recover(); rv != nil {
			log.Printf("[lua] panel section %q render panic: %v", s.ID, rv)
		}
	}()
	if err := s.plugin.L.CallByParam(glua.P{
		Fn:      s.renderFn,
		NRet:    1,
		Protect: true,
	}, glua.LNumber(w), glua.LNumber(h)); err != nil {
		log.Printf("[lua] panel section %q render error: %v", s.ID, err)
		return ""
	}
	result := s.plugin.L.Get(-1)
	s.plugin.L.Pop(1)
	if str, ok := result.(glua.LString); ok {
		return string(str)
	}
	return ""
}

// Render returns the section content at (w, h). Public interface for TUI consumers.
func (s *SectionDef) Render(w, h int) string { return s.CallRender(w, h) }

// PanelDef describes a panel created by claudio.win.new_panel.
type PanelDef struct {
	Position string // "left" | "right" | "bottom"
	Width    int
	Height   int
	Visible  bool
	Mu       sync.Mutex
	Sections []*SectionDef
}

// AddSection appends a section (caller must hold Mu).
func (p *PanelDef) AddSection(s *SectionDef) {
	p.Sections = append(p.Sections, s)
}

// RemoveSection removes the section with the given ID (caller must hold Mu).
func (p *PanelDef) RemoveSection(id string) bool {
	for i, s := range p.Sections {
		if s.ID == id {
			p.Sections = append(p.Sections[:i], p.Sections[i+1:]...)
			return true
		}
	}
	return false
}

// PanelRegistry is a thread-safe store for all panels created by Lua plugins.
type PanelRegistry struct {
	mu      sync.Mutex
	panels  []*PanelDef
	pending []*PanelDef
}

// NewPanelRegistry creates an empty registry.
func NewPanelRegistry() *PanelRegistry {
	return &PanelRegistry{}
}

// Register adds a panel to the live registry.
func (r *PanelRegistry) Register(p *PanelDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.panels = append(r.panels, p)
}

// Panels returns all panels at the given position (only visible ones).
func (r *PanelRegistry) Panels(position string) []*PanelDef {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*PanelDef
	for _, p := range r.panels {
		p.Mu.Lock()
		vis := p.Visible
		pos := p.Position
		p.Mu.Unlock()
		if pos == position && vis {
			out = append(out, p)
		}
	}
	return out
}

// AllPanels returns all registered panels regardless of position or visibility.
func (r *PanelRegistry) AllPanels() []*PanelDef {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*PanelDef, len(r.panels))
	copy(out, r.panels)
	return out
}

// FlushPending moves all pending panels into the live registry and returns them.
func (r *PanelRegistry) FlushPending() []*PanelDef {
	r.mu.Lock()
	defer r.mu.Unlock()
	flushed := r.pending
	r.panels = append(r.panels, r.pending...)
	r.pending = nil
	return flushed
}

// QueuePending adds a panel to the pending queue (before TUI is wired).
func (r *PanelRegistry) QueuePending(p *PanelDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = append(r.pending, p)
}

// PendingCount returns the number of queued panels (for testing).
func (r *PanelRegistry) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pending)
}
