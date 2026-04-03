package snippets

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

// ResolveContext analyzes the file content around the snippet insertion point
// and returns template variables like ReturnZeros and FuncName.
func ResolveContext(filePath, content string, offset int) map[string]string {
	ctx := make(map[string]string)

	ext := fileExt(filePath)
	switch ext {
	case "go":
		resolveGoContext(content, offset, ctx)
	case "py":
		resolvePythonContext(content, offset, ctx)
	case "ts", "tsx", "js", "jsx":
		resolveTSContext(content, offset, ctx)
	case "rs":
		resolveRustContext(content, offset, ctx)
	}

	return ctx
}

// resolveGoContext uses go/ast to find the enclosing function and extract return types.
func resolveGoContext(content string, offset int, ctx map[string]string) {
	fset := token.NewFileSet()
	// Wrap content in a valid Go file if it doesn't have a package declaration
	src := content
	needsWrap := !strings.Contains(content[:min(len(content), 200)], "package ")
	if needsWrap {
		src = "package _tmp\n" + content
		offset += len("package _tmp\n")
	}

	f, err := parser.ParseFile(fset, "", src, parser.AllErrors)
	if err != nil {
		// Best-effort: fall back to regex
		resolveGoContextRegex(content, offset, ctx)
		return
	}

	// Find the enclosing function declaration
	var enclosing *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		start := fset.Position(fd.Pos()).Offset
		end := fset.Position(fd.End()).Offset
		if offset >= start && offset <= end {
			enclosing = fd
		}
		return true
	})

	if enclosing == nil {
		return
	}

	ctx["FuncName"] = enclosing.Name.Name

	if enclosing.Type.Results == nil || len(enclosing.Type.Results.List) == 0 {
		ctx["ReturnZeros"] = ""
		return
	}

	var zeros []string
	for _, field := range enclosing.Type.Results.List {
		zero := goZeroValue(field.Type, src)
		// A field might have multiple names (rare in return types, but possible)
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			zeros = append(zeros, zero)
		}
	}

	// Remove the last element if it's "nil" (the error return) since
	// errw-style snippets add their own fmt.Errorf
	if len(zeros) > 0 && zeros[len(zeros)-1] == "nil" {
		ctx["ReturnZeros"] = strings.Join(zeros[:len(zeros)-1], ", ")
	} else {
		ctx["ReturnZeros"] = strings.Join(zeros, ", ")
	}
}

// goZeroValue returns the Go zero value for an AST type expression.
func goZeroValue(expr ast.Expr, src string) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return goZeroForName(t.Name)
	case *ast.StarExpr:
		return "nil" // pointer
	case *ast.ArrayType:
		return "nil" // slice
	case *ast.MapType:
		return "nil" // map
	case *ast.InterfaceType:
		return "nil"
	case *ast.ChanType:
		return "nil"
	case *ast.FuncType:
		return "nil"
	case *ast.SelectorExpr:
		// e.g., time.Duration → 0, context.Context → nil (interface)
		// For most selector exprs we can't know — default to zero struct
		typeName := exprToString(t, src)
		return typeName + "{}"
	case *ast.Ellipsis:
		return "nil" // variadic (shouldn't appear in return types but just in case)
	default:
		// Unknown type — try to extract the source text
		typeName := exprToString(expr, src)
		if typeName != "" {
			return typeName + "{}"
		}
		return "nil"
	}
}

// goZeroForName returns the zero value for a Go built-in type name.
func goZeroForName(name string) string {
	switch name {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64",
		"byte", "rune", "uintptr":
		return "0"
	case "string":
		return `""`
	case "bool":
		return "false"
	case "error":
		return "nil"
	default:
		// Named type (struct) — return TypeName{}
		return name + "{}"
	}
}

// exprToString extracts the source text for an AST expression.
func exprToString(expr ast.Expr, src string) string {
	// Simple approach: use the position info to extract from source
	// This works for most cases
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		inner := exprToString(t.X, src)
		return "*" + inner
	case *ast.SelectorExpr:
		x := exprToString(t.X, src)
		return x + "." + t.Sel.Name
	default:
		return ""
	}
}

// resolveGoContextRegex is a fallback when AST parsing fails.
var goFuncPattern = regexp.MustCompile(`func\s+(?:\([^)]*\)\s+)?(\w+)\s*\([^)]*\)\s*(?:\(([^)]*)\)|(\w+))`)

func resolveGoContextRegex(content string, offset int, ctx map[string]string) {
	// Find the last func declaration before offset
	matches := goFuncPattern.FindAllStringSubmatchIndex(content, -1)
	var bestMatch []int
	for _, m := range matches {
		if m[0] <= offset {
			bestMatch = m
		}
	}
	if bestMatch == nil {
		return
	}

	funcName := content[bestMatch[2]:bestMatch[3]]
	ctx["FuncName"] = funcName

	// Extract return types
	if bestMatch[4] >= 0 && bestMatch[5] >= 0 {
		// Multiple returns: (type1, type2, ...)
		returns := content[bestMatch[4]:bestMatch[5]]
		parts := strings.Split(returns, ",")
		var zeros []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			// Strip variable name if present (e.g., "err error" → "error")
			fields := strings.Fields(p)
			typeName := fields[len(fields)-1]
			zeros = append(zeros, goZeroForName(typeName))
		}
		if len(zeros) > 0 && zeros[len(zeros)-1] == "nil" {
			ctx["ReturnZeros"] = strings.Join(zeros[:len(zeros)-1], ", ")
		} else {
			ctx["ReturnZeros"] = strings.Join(zeros, ", ")
		}
	} else if bestMatch[6] >= 0 && bestMatch[7] >= 0 {
		// Single return type
		typeName := content[bestMatch[6]:bestMatch[7]]
		zero := goZeroForName(typeName)
		if zero == "nil" {
			ctx["ReturnZeros"] = ""
		} else {
			ctx["ReturnZeros"] = zero
		}
	}
}

// --- Python resolver ---

var pyFuncPattern = regexp.MustCompile(`def\s+(\w+)\s*\([^)]*\)(?:\s*->\s*(\S+))?\s*:`)

func resolvePythonContext(content string, offset int, ctx map[string]string) {
	matches := pyFuncPattern.FindAllStringSubmatchIndex(content, -1)
	var bestMatch []int
	for _, m := range matches {
		if m[0] <= offset {
			bestMatch = m
		}
	}
	if bestMatch == nil {
		return
	}
	ctx["FuncName"] = content[bestMatch[2]:bestMatch[3]]
	if bestMatch[4] >= 0 && bestMatch[5] >= 0 {
		ctx["ReturnType"] = content[bestMatch[4]:bestMatch[5]]
	}
	ctx["ReturnZeros"] = "None"
}

// --- TypeScript/JavaScript resolver ---

var tsFuncPattern = regexp.MustCompile(`(?:function|const|let|var)\s+(\w+).*?(?::\s*(\w[\w<>,\s|]*))?\s*[{=]`)

func resolveTSContext(content string, offset int, ctx map[string]string) {
	matches := tsFuncPattern.FindAllStringSubmatchIndex(content, -1)
	var bestMatch []int
	for _, m := range matches {
		if m[0] <= offset {
			bestMatch = m
		}
	}
	if bestMatch == nil {
		return
	}
	ctx["FuncName"] = content[bestMatch[2]:bestMatch[3]]
	if bestMatch[4] >= 0 && bestMatch[5] >= 0 {
		ctx["ReturnType"] = content[bestMatch[4]:bestMatch[5]]
	}
	ctx["ReturnZeros"] = "undefined"
}

// --- Rust resolver ---

var rustFuncPattern = regexp.MustCompile(`fn\s+(\w+)\s*(?:<[^>]*>)?\s*\([^)]*\)\s*(?:->\s*(.+?))\s*[{w]`)

func resolveRustContext(content string, offset int, ctx map[string]string) {
	matches := rustFuncPattern.FindAllStringSubmatchIndex(content, -1)
	var bestMatch []int
	for _, m := range matches {
		if m[0] <= offset {
			bestMatch = m
		}
	}
	if bestMatch == nil {
		return
	}
	ctx["FuncName"] = content[bestMatch[2]:bestMatch[3]]
	if bestMatch[4] >= 0 && bestMatch[5] >= 0 {
		ctx["ReturnType"] = strings.TrimSpace(content[bestMatch[4]:bestMatch[5]])
	}
	ctx["ReturnZeros"] = "Default::default()"
}
