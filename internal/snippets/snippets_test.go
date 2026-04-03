package snippets

import (
	"strings"
	"testing"
)

func TestExpand_Disabled(t *testing.T) {
	cfg := &Config{Enabled: false, Snippets: []SnippetDef{
		{Name: "test", Params: []string{"name"}, Template: "func Test{{.name}}(t *testing.T) {}"},
	}}
	input := "~test(Foo)"
	got := Expand(cfg, "foo.go", input)
	if got != input {
		t.Errorf("expected no expansion when disabled, got %q", got)
	}
}

func TestExpand_NilConfig(t *testing.T) {
	input := "~test(Foo)"
	got := Expand(nil, "foo.go", input)
	if got != input {
		t.Errorf("expected no expansion with nil config, got %q", got)
	}
}

func TestExpand_BasicTemplate(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Snippets: []SnippetDef{
			{
				Name:     "test",
				Params:   []string{"name"},
				Template: "func Test{{.name}}(t *testing.T) {\n\t// TODO\n}",
			},
		},
	}
	input := "~test(GetUser)"
	got := Expand(cfg, "foo.go", input)
	expected := "func TestGetUser(t *testing.T) {\n\t// TODO\n}"
	if got != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
	}
}

func TestExpand_LanguageFilter(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Snippets: []SnippetDef{
			{
				Name:     "test",
				Params:   []string{"name"},
				Lang:     "go",
				Template: "func Test{{.name}}(t *testing.T) {}",
			},
		},
	}
	// Should NOT expand in a .py file
	input := "~test(Foo)"
	got := Expand(cfg, "foo.py", input)
	if got != input {
		t.Errorf("expected no expansion for wrong language, got %q", got)
	}

	// Should expand in a .go file
	got = Expand(cfg, "foo.go", input)
	if got == input {
		t.Error("expected expansion for matching language")
	}
}

func TestExpand_UnknownSnippet(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Snippets: []SnippetDef{
			{Name: "test", Params: []string{"name"}, Template: "Test{{.name}}"},
		},
	}
	input := "~unknown(Foo)"
	got := Expand(cfg, "foo.go", input)
	if got != input {
		t.Errorf("expected unknown snippet to pass through, got %q", got)
	}
}

func TestExpand_NestedParens(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Snippets: []SnippetDef{
			{
				Name:     "errw",
				Params:   []string{"call", "msg"},
				Template: "{{.result}}, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, fmt.Errorf(\"{{.msg}}: %w\", err)\n}",
			},
		},
	}
	input := `package main

func GetUser(id int) (*User, error) {
	~errw(db.Query(ctx, "SELECT * FROM users WHERE id = ?", id), "query user")
	return user, nil
}
`
	got := Expand(cfg, "main.go", input)
	if strings.Contains(got, "~errw") {
		t.Errorf("snippet was not expanded: %s", got)
	}
	if !strings.Contains(got, "db.Query(ctx, \"SELECT * FROM users WHERE id = ?\", id)") {
		t.Errorf("args not parsed correctly: %s", got)
	}
	if !strings.Contains(got, "if err != nil") {
		t.Errorf("template not expanded: %s", got)
	}
}

func TestExpand_MultipleSnippets(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Snippets: []SnippetDef{
			{Name: "a", Params: []string{"x"}, Template: "A({{.x}})"},
			{Name: "b", Params: []string{"y"}, Template: "B({{.y}})"},
		},
	}
	input := "~a(1) then ~b(2)"
	got := Expand(cfg, "foo.go", input)
	if !strings.Contains(got, "A(1)") {
		t.Errorf("first snippet not expanded: %s", got)
	}
	if !strings.Contains(got, "B(2)") {
		t.Errorf("second snippet not expanded: %s", got)
	}
}

func TestExpand_GoContextReturnTypes(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Snippets: []SnippetDef{
			{
				Name:     "errw",
				Params:   []string{"call", "msg"},
				Lang:     "go",
				Template: "r, err := {{.call}}\nif err != nil {\n\treturn {{.ReturnZeros}}, fmt.Errorf(\"{{.msg}}: %w\", err)\n}",
			},
		},
	}

	tests := []struct {
		name     string
		input    string
		wantZero string
	}{
		{
			name: "pointer return",
			input: `package main

func GetUser(id int) (*User, error) {
	~errw(db.Find(id), "find")
	return nil, nil
}`,
			wantZero: "return nil,",
		},
		{
			name: "int return",
			input: `package main

func Count() (int, error) {
	~errw(db.Count(), "count")
	return 0, nil
}`,
			wantZero: "return 0,",
		},
		{
			name: "string return",
			input: `package main

func Name() (string, error) {
	~errw(db.Name(), "name")
	return "", nil
}`,
			wantZero: `return "",`,
		},
		{
			name: "multi return",
			input: `package main

func Pair() (string, int, error) {
	~errw(db.Pair(), "pair")
	return "", 0, nil
}`,
			wantZero: `return "", 0,`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Expand(cfg, "main.go", tt.input)
			if !strings.Contains(got, tt.wantZero) {
				t.Errorf("expected %q in output:\n%s", tt.wantZero, got)
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a, b", 2},
		{`db.Query(ctx, "hello"), "msg"`, 2},
		{`a`, 1},
		{`a, b, c`, 3},
		{`fn(a, b), "x"`, 2},
	}
	for _, tt := range tests {
		args := parseArgs(tt.input)
		if len(args) != tt.want {
			t.Errorf("parseArgs(%q) = %d args %v, want %d", tt.input, len(args), args, tt.want)
		}
	}
}

func TestFindClosingParen(t *testing.T) {
	tests := []struct {
		input string
		pos   int
		want  int
	}{
		{"(abc)", 1, 4},
		{"(a(b)c)", 1, 6},
		{`(a, ")")`, 1, 7},
		{"(no close", 1, -1},
	}
	for _, tt := range tests {
		got := findClosingParen(tt.input, tt.pos)
		if got != tt.want {
			t.Errorf("findClosingParen(%q, %d) = %d, want %d", tt.input, tt.pos, got, tt.want)
		}
	}
}

func TestFileExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"foo.go", "go"},
		{"/path/to/bar.py", "py"},
		{"no-ext", ""},
		{"dir/file.test.ts", "ts"},
	}
	for _, tt := range tests {
		got := fileExt(tt.path)
		if got != tt.want {
			t.Errorf("fileExt(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestForSystemPrompt(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Snippets: []SnippetDef{
			{Name: "errw", Params: []string{"call", "msg"}, Lang: "go", Template: "err handling..."},
			{Name: "test", Params: []string{"name"}, Template: "test scaffold..."},
		},
	}
	got := ForSystemPrompt(cfg)
	if !strings.Contains(got, "~errw(call, msg)") {
		t.Error("expected errw snippet in prompt")
	}
	if !strings.Contains(got, "## go") {
		t.Error("expected go language group header")
	}
	if !strings.Contains(got, "~test(name)") {
		t.Error("expected test snippet in prompt")
	}
	if !strings.Contains(got, "## All languages") {
		t.Error("expected 'All languages' group for untagged snippets")
	}
	if !strings.Contains(got, "## Example") {
		t.Error("expected example section")
	}
	if !strings.Contains(got, "do NOT fill them in yourself") {
		t.Error("expected context variable instruction")
	}
}

func TestForSystemPrompt_Disabled(t *testing.T) {
	cfg := &Config{Enabled: false, Snippets: []SnippetDef{
		{Name: "test", Params: []string{"name"}, Template: "..."},
	}}
	if got := ForSystemPrompt(cfg); got != "" {
		t.Errorf("expected empty string when disabled, got %q", got)
	}
}

func TestResolveGoContext_RegexFallback(t *testing.T) {
	// Invalid Go that won't parse with go/ast — should fall back to regex
	content := `func Broken( GetUser(id int) (*User, error) {
	something
}`
	ctx := make(map[string]string)
	resolveGoContext(content, 50, ctx)
	// Should at least not panic — the regex fallback is best-effort
}
