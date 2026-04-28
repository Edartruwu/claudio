package sidebar

import (
	"sort"
	"sync"
)

// BlockRegistry holds all sidebar blocks registered at runtime.
// It is safe for concurrent use.
type BlockRegistry struct {
	mu     sync.RWMutex
	blocks []Block
}

// NewBlockRegistry creates an empty registry.
func NewBlockRegistry() *BlockRegistry { return &BlockRegistry{} }

// Register adds a block to the registry.
func (r *BlockRegistry) Register(b Block) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blocks = append(r.blocks, b)
}

// Blocks returns a sorted copy of all registered blocks, weight descending.
func (r *BlockRegistry) Blocks() []Block {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Block, len(r.blocks))
	copy(out, r.blocks)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Weight() > out[j].Weight()
	})
	return out
}
