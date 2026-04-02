package styles

// GruvboxGlamourJSON returns a glamour-compatible JSON style using gruvbox dark colors.
func GruvboxGlamourJSON() []byte {
	return []byte(gruvboxJSON)
}

const gruvboxJSON = `{
  "document": {
    "block_prefix": "\n",
    "block_suffix": "\n",
    "color": "#ebdbb2",
    "margin": 2
  },
  "block_quote": {
    "indent": 1,
    "indent_token": "│ ",
    "color": "#928374"
  },
  "paragraph": {},
  "list": {
    "level_indent": 2
  },
  "heading": {
    "block_suffix": "\n",
    "color": "#fabd2f",
    "bold": true
  },
  "h1": {
    "prefix": " ",
    "suffix": " ",
    "color": "#282828",
    "background_color": "#fabd2f",
    "bold": true
  },
  "h2": {
    "prefix": "## ",
    "color": "#b8bb26",
    "bold": true
  },
  "h3": {
    "prefix": "### ",
    "color": "#8ec07c",
    "bold": true
  },
  "h4": {
    "prefix": "#### ",
    "color": "#83a598"
  },
  "h5": {
    "prefix": "##### ",
    "color": "#d3869b"
  },
  "h6": {
    "prefix": "###### ",
    "color": "#928374"
  },
  "text": {},
  "strikethrough": {
    "crossed_out": true
  },
  "emph": {
    "italic": true,
    "color": "#bdae93"
  },
  "strong": {
    "bold": true,
    "color": "#ebdbb2"
  },
  "hr": {
    "color": "#504945",
    "format": "\n────────\n"
  },
  "item": {
    "block_prefix": "• "
  },
  "enumeration": {
    "block_prefix": ". "
  },
  "task": {
    "ticked": "[✓] ",
    "unticked": "[ ] "
  },
  "link": {
    "color": "#83a598",
    "underline": true
  },
  "link_text": {
    "color": "#d3869b",
    "bold": true
  },
  "image": {
    "color": "#fe8019",
    "underline": true
  },
  "image_text": {
    "color": "#928374"
  },
  "code": {
    "prefix": " ",
    "suffix": " ",
    "color": "#fe8019",
    "background_color": "#3c3836"
  },
  "code_block": {
    "color": "#ebdbb2",
    "margin": 2,
    "chroma": {
      "text":                { "color": "#ebdbb2" },
      "error":               { "color": "#fb4934" },
      "comment":             { "color": "#928374", "italic": true },
      "comment_preproc":     { "color": "#8ec07c" },
      "keyword":             { "color": "#fb4934" },
      "keyword_reserved":    { "color": "#fb4934" },
      "keyword_namespace":   { "color": "#fe8019" },
      "keyword_type":        { "color": "#fabd2f" },
      "keyword_declaration": { "color": "#fb4934" },
      "operator":            { "color": "#ebdbb2" },
      "punctuation":         { "color": "#ebdbb2" },
      "name":                { "color": "#ebdbb2" },
      "name_builtin":        { "color": "#fabd2f" },
      "name_tag":            { "color": "#fb4934" },
      "name_attribute":      { "color": "#b8bb26" },
      "name_class":          { "color": "#fabd2f", "bold": true },
      "name_constant":       { "color": "#d3869b" },
      "name_decorator":      { "color": "#8ec07c" },
      "name_exception":      { "color": "#fb4934", "bold": true },
      "name_function":       { "color": "#b8bb26" },
      "name_other":          {},
      "literal":             {},
      "literal_number":      { "color": "#d3869b" },
      "literal_date":        {},
      "literal_string":      { "color": "#b8bb26" },
      "literal_string_escape": { "color": "#fe8019" },
      "generic_deleted":     { "color": "#fb4934" },
      "generic_emph":        { "italic": true },
      "generic_inserted":    { "color": "#b8bb26" },
      "generic_strong":      { "bold": true },
      "generic_subheading":  { "color": "#928374" },
      "background":          { "background_color": "#282828" }
    }
  },
  "table": {},
  "definition_list": {},
  "definition_term": {},
  "definition_description": {
    "block_prefix": "\n> "
  },
  "html_block": {},
  "html_span": {}
}`
