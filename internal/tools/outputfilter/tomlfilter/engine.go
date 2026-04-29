// Package tomlfilter implements a declarative TOML-driven output filter engine.
// Users can define filters in .claudio/filters.toml (project-local) or
// ~/.config/claudio/filters.toml (user-global) without recompiling.
package tomlfilter

import (
	"fmt"
	"regexp"
	"strings"
)

// ReplaceRule defines a regex substitution applied line-by-line.
type ReplaceRule struct {
	Pattern     string `toml:"pattern"`
	Replacement string `toml:"replacement"`
}

// MatchOutputRule defines a pattern match against the full output blob.
// If Pattern matches (and Unless does NOT match), the rule short-circuits
// and returns Message immediately.
type MatchOutputRule struct {
	Pattern string `toml:"pattern"`
	Message string `toml:"message"`
	Unless  string `toml:"unless,omitempty"`
}

// FilterDef is the TOML representation of a single filter definition.
type FilterDef struct {
	Description        string            `toml:"description"`
	MatchCommand       string            `toml:"match_command"`
	StripAnsi          bool              `toml:"strip_ansi"`
	Replace            []ReplaceRule     `toml:"replace"`
	MatchOutput        []MatchOutputRule `toml:"match_output"`
	StripLinesMatching []string          `toml:"strip_lines_matching"`
	KeepLinesMatching  []string          `toml:"keep_lines_matching"`
	TruncateLinesAt    *int              `toml:"truncate_lines_at"`
	HeadLines          *int              `toml:"head_lines"`
	TailLines          *int              `toml:"tail_lines"`
	MaxLines           *int              `toml:"max_lines"`
	OnEmpty            string            `toml:"on_empty"`
}

// filterFile is the top-level TOML structure for a filter file.
type filterFile struct {
	SchemaVersion int                  `toml:"schema_version"`
	Filters       map[string]FilterDef `toml:"filters"`
}

// compiledFilter holds a FilterDef with pre-compiled regexes for fast matching.
type compiledFilter struct {
	name           string
	def            FilterDef
	commandRe      *regexp.Regexp
	replaceRules   []compiledReplace
	matchOutput    []compiledMatchOutput
	stripLinesRes  []*regexp.Regexp
	keepLinesRes   []*regexp.Regexp
}

type compiledReplace struct {
	re          *regexp.Regexp
	replacement string
}

type compiledMatchOutput struct {
	re       *regexp.Regexp
	message  string
	unlessRe *regexp.Regexp
}

// ansiRe matches ANSI escape sequences.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// compile pre-compiles all regexes in a FilterDef. Returns an error if any
// regex is invalid.
func compile(name string, def FilterDef) (compiledFilter, error) {
	cf := compiledFilter{name: name, def: def}

	cmdRe, err := regexp.Compile(def.MatchCommand)
	if err != nil {
		return cf, fmt.Errorf("match_command: %w", err)
	}
	cf.commandRe = cmdRe

	for i, r := range def.Replace {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return cf, fmt.Errorf("replace[%d].pattern: %w", i, err)
		}
		cf.replaceRules = append(cf.replaceRules, compiledReplace{re: re, replacement: r.Replacement})
	}

	for i, m := range def.MatchOutput {
		re, err := regexp.Compile(m.Pattern)
		if err != nil {
			return cf, fmt.Errorf("match_output[%d].pattern: %w", i, err)
		}
		cm := compiledMatchOutput{re: re, message: m.Message}
		if m.Unless != "" {
			unlessRe, err := regexp.Compile(m.Unless)
			if err != nil {
				return cf, fmt.Errorf("match_output[%d].unless: %w", i, err)
			}
			cm.unlessRe = unlessRe
		}
		cf.matchOutput = append(cf.matchOutput, cm)
	}

	for i, p := range def.StripLinesMatching {
		re, err := regexp.Compile(p)
		if err != nil {
			return cf, fmt.Errorf("strip_lines_matching[%d]: %w", i, err)
		}
		cf.stripLinesRes = append(cf.stripLinesRes, re)
	}

	for i, p := range def.KeepLinesMatching {
		re, err := regexp.Compile(p)
		if err != nil {
			return cf, fmt.Errorf("keep_lines_matching[%d]: %w", i, err)
		}
		cf.keepLinesRes = append(cf.keepLinesRes, re)
	}

	return cf, nil
}

// apply runs the 8-stage pipeline on the output. It assumes the command
// has already been matched by commandRe.
func (cf *compiledFilter) apply(output string) string {
	result := output

	// Stage 1: strip_ansi
	if cf.def.StripAnsi {
		result = ansiRe.ReplaceAllString(result, "")
	}

	// Stage 2: replace (line-by-line, chained)
	if len(cf.replaceRules) > 0 {
		lines := strings.Split(result, "\n")
		for i, line := range lines {
			for _, r := range cf.replaceRules {
				line = r.re.ReplaceAllString(line, r.replacement)
			}
			lines[i] = line
		}
		result = strings.Join(lines, "\n")
	}

	// Stage 3: match_output (check full blob, short-circuit)
	for _, m := range cf.matchOutput {
		if m.re.MatchString(result) {
			if m.unlessRe != nil && m.unlessRe.MatchString(result) {
				continue
			}
			return m.message
		}
	}

	// Stage 4: strip_lines_matching / keep_lines_matching
	if len(cf.stripLinesRes) > 0 {
		lines := strings.Split(result, "\n")
		var kept []string
		for _, line := range lines {
			strip := false
			for _, re := range cf.stripLinesRes {
				if re.MatchString(line) {
					strip = true
					break
				}
			}
			if !strip {
				kept = append(kept, line)
			}
		}
		result = strings.Join(kept, "\n")
	} else if len(cf.keepLinesRes) > 0 {
		lines := strings.Split(result, "\n")
		var kept []string
		for _, line := range lines {
			for _, re := range cf.keepLinesRes {
				if re.MatchString(line) {
					kept = append(kept, line)
					break
				}
			}
		}
		result = strings.Join(kept, "\n")
	}

	// Stage 5: truncate_lines_at
	if cf.def.TruncateLinesAt != nil {
		max := *cf.def.TruncateLinesAt
		lines := strings.Split(result, "\n")
		for i, line := range lines {
			runes := []rune(line)
			if len(runes) > max {
				lines[i] = string(runes[:max]) + "..."
			}
		}
		result = strings.Join(lines, "\n")
	}

	// Stage 6: head_lines
	if cf.def.HeadLines != nil {
		lines := strings.Split(result, "\n")
		n := *cf.def.HeadLines
		if n < len(lines) {
			lines = lines[:n]
		}
		result = strings.Join(lines, "\n")
	}

	// Stage 7: tail_lines (applied after head_lines)
	if cf.def.TailLines != nil {
		lines := strings.Split(result, "\n")
		n := *cf.def.TailLines
		if n < len(lines) {
			lines = lines[len(lines)-n:]
		}
		result = strings.Join(lines, "\n")
	}

	// Stage 8: max_lines (absolute cap, applied last)
	if cf.def.MaxLines != nil {
		lines := strings.Split(result, "\n")
		n := *cf.def.MaxLines
		if n < len(lines) {
			lines = lines[:n]
		}
		result = strings.Join(lines, "\n")
	}

	// on_empty: if result is empty after trimming, return fallback
	if cf.def.OnEmpty != "" && strings.TrimSpace(result) == "" {
		return cf.def.OnEmpty
	}

	return result
}

// ApplyPipeline compiles a FilterDef and runs the 8-stage pipeline on output.
// The match_command field is not checked — the caller is responsible for matching.
// Returns an error if any regex in the def fails to compile.
func ApplyPipeline(def FilterDef, output string) (string, error) {
	cf, err := compile("lua", def)
	if err != nil {
		return output, err
	}
	return cf.apply(output), nil
}
