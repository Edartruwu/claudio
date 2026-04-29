package picker

// LayoutStrategy controls how the picker renders its three panes.
type LayoutStrategy string

const (
	// LayoutHorizontal splits the screen: results (60%) left, preview (40%) right.
	LayoutHorizontal LayoutStrategy = "horizontal"

	// LayoutVertical stacks preview (top 40%) → results → prompt (bottom).
	LayoutVertical LayoutStrategy = "vertical"

	// LayoutDropdown renders a compact centered modal with no preview pane.
	LayoutDropdown LayoutStrategy = "dropdown"

	// LayoutIvy renders a full-width bottom overlay (~30% height), telescope ivy-like.
	LayoutIvy LayoutStrategy = "ivy"
)

// Config holds all configuration for a picker instance.
type Config struct {
	// Title displayed at the top of the results pane (optional).
	Title string

	// Finder supplies entries asynchronously via a channel.
	Finder Finder

	// Sorter ranks entries on each keystroke. Defaults to FuzzySorter when nil.
	Sorter Sorter

	// Previewer renders the preview pane for the highlighted entry.
	// When nil, no preview pane is shown.
	Previewer Previewer

	// Layout controls the pane arrangement. Defaults to LayoutHorizontal.
	Layout LayoutStrategy

	// OnSelect is called when the user presses Enter on a single entry.
	OnSelect func(entry Entry)

	// OnMultiSelect is called when the user has Tab-selected multiple entries
	// and then presses Enter.
	OnMultiSelect func(entries []Entry)
}
