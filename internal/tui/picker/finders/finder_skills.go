package finders

import (
	"context"

	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/tui/picker"
)

// skillFinder emits entries for every skill in a skills.Registry.
type skillFinder struct {
	reg *skills.Registry
}

// NewSkillFinder returns a Finder over all skills in reg.
// Each entry: Display = "name — description", Ordinal = name,
// Meta["name"] = name, Meta["source"] = source string.
func NewSkillFinder(reg *skills.Registry) picker.Finder {
	return &skillFinder{reg: reg}
}

func (f *skillFinder) Find(ctx context.Context) <-chan picker.Entry {
	all := f.reg.All()
	ch := make(chan picker.Entry, len(all))
	go func() {
		defer close(ch)
		for _, s := range all {
			display := s.Name
			if s.Description != "" {
				display += " — " + s.Description
			}
			e := picker.Entry{
				Value:   s,
				Display: display,
				Ordinal: s.Name,
				Meta: map[string]any{
					"name":        s.Name,
					"description": s.Description,
					"source":      s.Source,
				},
			}
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

func (f *skillFinder) Close() {}
