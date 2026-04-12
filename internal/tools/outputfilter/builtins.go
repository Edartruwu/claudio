package outputfilter

import "embed"

//go:embed filters/*.toml
var builtinFilters embed.FS

// BuiltinFiltersFS returns the embedded filesystem containing built-in
// filter TOML definitions.
func BuiltinFiltersFS() embed.FS {
	return builtinFilters
}
