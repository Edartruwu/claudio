package sidebar

// LuaBlock wraps a Lua-registered block config into the Block interface.
// The renderFn is a closure over the Lua SidebarBlockDef.CallRender method,
// keeping this package free of any dependency on the lua package.
type LuaBlock struct {
	name      string
	title     string
	weight    int
	minHeight int
	renderFn  func(w, h int) string
}

// NewLuaBlock creates a LuaBlock from the given configuration.
func NewLuaBlock(name, title string, weight, minHeight int, render func(w, h int) string) *LuaBlock {
	return &LuaBlock{
		name:      name,
		title:     title,
		weight:    weight,
		minHeight: minHeight,
		renderFn:  render,
	}
}

func (b *LuaBlock) Title() string          { return b.title }
func (b *LuaBlock) Weight() int            { return b.weight }
func (b *LuaBlock) MinHeight() int         { return b.minHeight }
func (b *LuaBlock) Render(w, h int) string { return b.renderFn(w, h) }
