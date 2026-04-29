package finders

import (
	"context"

	"github.com/Abraxas-365/claudio/internal/cli/commands"
	"github.com/Abraxas-365/claudio/internal/tui/picker"
)

// commandFinder emits entries for every registered command.
type commandFinder struct {
	reg *commands.Registry
}

// NewCommandFinder returns a Finder over all commands in reg.
// Each entry: Display = "/name — description", Ordinal = name.
func NewCommandFinder(reg *commands.Registry) picker.Finder {
	return &commandFinder{reg: reg}
}

func (f *commandFinder) Find(ctx context.Context) <-chan picker.Entry {
	cmds := f.reg.ListCommands()
	ch := make(chan picker.Entry, len(cmds))
	go func() {
		defer close(ch)
		for _, cmd := range cmds {
			display := "/" + cmd.Name
			if cmd.Description != "" {
				display += " — " + cmd.Description
			}
			e := picker.Entry{
				Value:   cmd,
				Display: display,
				Ordinal: cmd.Name,
				Meta: map[string]any{
					"name":        cmd.Name,
					"description": cmd.Description,
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

func (f *commandFinder) Close() {}
