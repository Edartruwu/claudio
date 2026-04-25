// Package config implements the configuration viewer/editor side panel.
// It supports editing both project-level (.claudio/settings.json) and
// global (~/.claudio/settings.json) settings, with visual distinction.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// ConfigChangedMsg is sent when a setting is toggled in the panel.
// The root model should apply the change to the live session.
type ConfigChangedMsg struct {
	Key   string // setting key that changed
	Value string // new value
}

// InsertCommandMsg is sent when the user selects a model shortcut in the panel.
// The root model should insert the command into the prompt text box.
type InsertCommandMsg struct {
	Command string // e.g. "/sonnet"
}

// Scope identifies where a setting lives.
type Scope int

const (
	ScopeGlobal  Scope = iota // ~/.claudio/settings.json
	ScopeProject              // .claudio/settings.json
)

// Package-level styles for the config panel (avoids per-frame allocation).
var (
	cfgNoProject    = lipgloss.NewStyle().Foreground(styles.Dim).Italic(true)
	cfgKeyStyle     = lipgloss.NewStyle().Foreground(styles.Aqua)
	cfgValStyle     = lipgloss.NewStyle().Foreground(styles.Text)
	cfgValDim       = lipgloss.NewStyle().Foreground(styles.Dim)
	cfgEditHint     = lipgloss.NewStyle().Foreground(styles.Warning)
	cfgProjBadge    = lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Orange)
	cfgGlobalBadge  = lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Secondary)
	cfgAllowBadge   = lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Success)
	cfgDenyBadge    = lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Error)
	cfgRuleTool     = lipgloss.NewStyle().Foreground(styles.Warning).Bold(true)
	cfgRulePattern  = lipgloss.NewStyle().Foreground(styles.Dim)
	cfgSectionHeader = lipgloss.NewStyle().Foreground(styles.Muted).Bold(true)

	cfgActivePill   = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	cfgInactivePill = lipgloss.NewStyle().Foreground(styles.Muted)
	cfgTabHint      = lipgloss.NewStyle().Foreground(styles.Dim).Italic(true)
)

// configEntry represents a single setting for display.
type configEntry struct {
	Key       string
	Value     string
	Editable  bool   // can be toggled/cycled with enter
	EditType  string // "bool", "cycle"
	Source    Scope  // where this value comes from
	Deletable bool   // can be deleted with d
	RuleIndex int    // index into PermissionRules (-1 if not a rule)
}

// Panel is the configuration viewer/editor side panel.
type Panel struct {
	merged  *config.Settings // merged view (what's in effect)
	project *config.Settings // project-level overrides (nil if no .claudio/)
	global  *config.Settings // global user settings

	projectPath string // path to .claudio/settings.json (empty if not present)
	globalPath  string // path to ~/.claudio/settings.json

	active     bool
	width      int
	height     int
	cursor     int
	entries    []configEntry
	editScope  Scope // which scope we're editing (toggle with tab)
	hasProject bool  // whether .claudio/ exists
}

// New creates a new configuration panel.
// cfg is the merged config. The panel will load project and global configs separately.
func New(cfg *config.Settings) *Panel {
	paths := config.GetPaths()
	cwd, _ := os.Getwd()
	projectRoot := config.FindGitRoot(cwd)

	p := &Panel{
		merged:     cfg,
		globalPath: paths.Settings,
		editScope:  ScopeProject, // default to editing project if available
	}

	// Load global settings
	p.global = loadSettingsFile(paths.Settings)

	// Load project settings
	projectSettings := filepath.Join(projectRoot, ".claudio", "settings.json")
	if _, err := os.Stat(filepath.Join(projectRoot, ".claudio")); err == nil {
		p.hasProject = true
		p.projectPath = projectSettings
		p.project = loadSettingsFile(projectSettings)
		p.editScope = ScopeProject
	} else {
		p.editScope = ScopeGlobal
	}

	return p
}

// loadSettingsFile reads a settings JSON file, returning defaults if missing.
func loadSettingsFile(path string) *config.Settings {
	data, err := os.ReadFile(path)
	if err != nil {
		return &config.Settings{}
	}
	var s config.Settings
	json.Unmarshal(data, &s)
	return &s
}

func (p *Panel) IsActive() bool { return p.active }

func (p *Panel) Activate() {
	p.active = true
	p.cursor = 0
	p.buildEntries()
}

func (p *Panel) Deactivate() { p.active = false }

func (p *Panel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *Panel) buildEntries() {
	p.entries = nil
	m := p.merged

	addE := func(key, val, editType string, src Scope) {
		p.entries = append(p.entries, configEntry{
			Key: key, Value: val, Editable: true, EditType: editType, Source: src,
		})
	}
	addR := func(key, val string, src Scope) {
		p.entries = append(p.entries, configEntry{
			Key: key, Value: val, Source: src,
		})
	}

	addE("model", valOrDefault(m.Model, "claude-sonnet-4-6"), "cycle", p.source("model"))
	addR("smallModel", valOrDefault(m.SmallModel, "claude-haiku-4-5"), p.source("smallModel"))
	addE("permissionMode", valOrDefault(m.PermissionMode, "default"), "cycle", p.source("permissionMode"))
	addE("autoCompact", fmt.Sprintf("%v", m.AutoCompact), "bool", p.source("autoCompact"))
	addE("compactMode", valOrDefault(m.CompactMode, "auto"), "cycle", p.source("compactMode"))
	addE("compactKeepN", fmt.Sprintf("%d", m.GetCompactKeepN()), "int", p.source("compactKeepN"))
	addE("sessionPersist", fmt.Sprintf("%v", m.SessionPersist), "bool", p.source("sessionPersist"))
	addR("hookProfile", valOrDefault(m.HookProfile, "standard"), p.source("hookProfile"))
	addE("caveman", fmt.Sprintf("%v", m.CavemanEnabled()), "bool", p.source("caveman"))

	// Memory settings
	addE("autoMemoryExtract", fmt.Sprintf("%v", m.IsAutoMemoryExtract()), "bool", p.source("autoMemoryExtract"))
	addE("memorySelection", m.GetMemorySelection(), "cycle", p.source("memorySelection"))

	if m.MaxBudget > 0 {
		addR("maxBudget", fmt.Sprintf("$%.2f", m.MaxBudget), p.source("maxBudget"))
	} else {
		addR("maxBudget", "unlimited", ScopeGlobal)
	}

	addE("outputFilter", fmt.Sprintf("%v", m.OutputFilter), "bool", p.source("outputFilter"))

	if m.OutputStyle != "" {
		addE("outputStyle", m.OutputStyle, "cycle", p.source("outputStyle"))
	} else {
		addE("outputStyle", "normal", "cycle", ScopeGlobal)
	}

	addE("codeFilterLevel", valOrDefault(m.CodeFilterLevel, "minimal"), "cycle", p.source("codeFilterLevel"))

	if len(m.DenyTools) > 0 {
		addR("denyTools", strings.Join(m.DenyTools, ", "), p.source("denyTools"))
	} else {
		addR("denyTools", "(none)", ScopeGlobal)
	}

	// Providers section
	if len(m.Providers) > 0 {
		p.entries = append(p.entries, configEntry{
			Key: "providers", Value: fmt.Sprintf("%d configured", len(m.Providers)),
			RuleIndex: -1,
		})
		providerNames := make([]string, 0, len(m.Providers))
		for name := range m.Providers {
			providerNames = append(providerNames, name)
		}
		sort.Strings(providerNames)
		for _, name := range providerNames {
			pc := m.Providers[name]
			p.entries = append(p.entries, configEntry{
				Key: "  " + name, Value: fmt.Sprintf("%s (%s)", pc.APIBase, pc.Type),
				Source: p.source("providers"), RuleIndex: -1,
			})
			shortcuts := make([]string, 0, len(pc.Models))
			for shortcut := range pc.Models {
				shortcuts = append(shortcuts, shortcut)
			}
			sort.Strings(shortcuts)
			for _, shortcut := range shortcuts {
				p.entries = append(p.entries, configEntry{
					Key: "    /" + shortcut, Value: pc.Models[shortcut],
					Source: p.source("providers"), RuleIndex: -1,
				})
			}
		}
	}

	// Permission rules (deletable with d)
	if len(m.PermissionRules) > 0 {
		p.entries = append(p.entries, configEntry{
			Key: "permissions", Value: fmt.Sprintf("%d rules", len(m.PermissionRules)),
			RuleIndex: -1,
		})
		for i, rule := range m.PermissionRules {
			pattern := shortenPath(rule.Pattern)
			p.entries = append(p.entries, configEntry{
				Key: rule.Tool, Value: pattern, Source: p.ruleSource(i),
				Deletable: true, RuleIndex: i,
				EditType:  rule.Behavior, // stash behavior in EditType for rendering
			})
		}
	}
}

// ruleSource determines if a permission rule came from project or global config.
func (p *Panel) ruleSource(idx int) Scope {
	if p.project != nil && idx < len(p.project.PermissionRules) {
		return ScopeProject
	}
	return ScopeGlobal
}

// source determines which scope a setting's value came from.
func (p *Panel) source(key string) Scope {
	if p.project == nil {
		return ScopeGlobal
	}
	// Check if the project file has a non-zero value for this key
	switch key {
	case "model":
		if p.project.Model != "" {
			return ScopeProject
		}
	case "smallModel":
		if p.project.SmallModel != "" {
			return ScopeProject
		}
	case "permissionMode":
		if p.project.PermissionMode != "" {
			return ScopeProject
		}
	case "caveman":
		if p.project.Caveman != nil {
			return ScopeProject
		}
	case "autoMemoryExtract":
		if p.project.AutoMemoryExtract != nil {
			return ScopeProject
		}
	case "memorySelection":
		if p.project.MemorySelection != "" {
			return ScopeProject
		}
	case "compactMode":
		if p.project.CompactMode != "" {
			return ScopeProject
		}
	case "compactKeepN":
		if p.project.CompactKeepN > 0 {
			return ScopeProject
		}
	case "outputFilter":
		if p.projectPath != "" {
			if data, err := os.ReadFile(p.projectPath); err == nil {
				var raw map[string]json.RawMessage
				if json.Unmarshal(data, &raw) == nil {
					if _, ok := raw["outputFilter"]; ok {
						return ScopeProject
					}
				}
			}
		}
	case "outputStyle":
		if p.project.OutputStyle != "" {
			return ScopeProject
		}
	case "codeFilterLevel":
		if p.project.CodeFilterLevel != "" {
			return ScopeProject
		}
	case "providers":
		if len(p.project.Providers) > 0 {
			return ScopeProject
		}
	}
	return ScopeGlobal
}

func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "j", "down":
		if p.cursor < len(p.entries)-1 {
			p.cursor++
		}
		return nil, true
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, true
	case "G":
		p.cursor = max(0, len(p.entries)-1)
		return nil, true
	case "g":
		p.cursor = 0
		return nil, true
	case "d":
		// Delete permission rule
		if p.cursor < len(p.entries) && p.entries[p.cursor].Deletable {
			idx := p.entries[p.cursor].RuleIndex
			if idx >= 0 && idx < len(p.merged.PermissionRules) {
				p.merged.PermissionRules = append(
					p.merged.PermissionRules[:idx],
					p.merged.PermissionRules[idx+1:]...,
				)
				p.savePermissionRules()
				p.buildEntries()
				if p.cursor >= len(p.entries) {
					p.cursor = max(0, len(p.entries)-1)
				}
			}
		}
		return nil, true
	case "tab":
		// Toggle edit scope between project and global
		if p.hasProject {
			if p.editScope == ScopeProject {
				p.editScope = ScopeGlobal
			} else {
				p.editScope = ScopeProject
			}
		}
		return nil, true
	case "enter", " ":
		if p.cursor < len(p.entries) {
			e := p.entries[p.cursor]
			// Model shortcut entries: insert command into prompt
			if strings.HasPrefix(e.Key, "    /") {
				cmd := strings.TrimSpace(e.Key)
				p.active = false
				return func() tea.Msg {
					return InsertCommandMsg{Command: cmd}
				}, true
			}
			if e.Editable {
				key, val := p.toggleEntry(p.cursor)
				p.buildEntries()
				return func() tea.Msg {
					return ConfigChangedMsg{Key: key, Value: val}
				}, true
			}
		}
		return nil, true
	}
	return nil, false
}

func (p *Panel) toggleEntry(idx int) (string, string) {
	e := p.entries[idx]
	p.ensureProjectConfig()
	target := p.project
	var newVal string

	switch e.Key {
	case "model":
		models := []string{"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5-20251001"}
		// Append provider model IDs so they can be cycled through
		for _, pc := range p.merged.Providers {
			for _, modelID := range pc.Models {
				models = append(models, modelID)
			}
		}
		target.Model = cycleValue(p.merged.Model, models, "claude-sonnet-4-6")
		newVal = target.Model
	case "autoCompact":
		target.AutoCompact = !p.merged.AutoCompact
		newVal = fmt.Sprintf("%v", target.AutoCompact)
	case "sessionPersist":
		target.SessionPersist = !p.merged.SessionPersist
		newVal = fmt.Sprintf("%v", target.SessionPersist)
	case "caveman":
		val := !p.merged.CavemanEnabled()
		target.Caveman = &val
		newVal = fmt.Sprintf("%v", val)
	case "autoMemoryExtract":
		val := !p.merged.IsAutoMemoryExtract()
		target.AutoMemoryExtract = &val
		newVal = fmt.Sprintf("%v", val)
	case "permissionMode":
		modes := []string{"default", "auto", "plan"}
		target.PermissionMode = cycleValue(p.merged.PermissionMode, modes, "default")
		newVal = target.PermissionMode
	case "memorySelection":
		modes := []string{"ai", "keyword", "none"}
		target.MemorySelection = cycleValue(p.merged.GetMemorySelection(), modes, "ai")
		newVal = target.MemorySelection
	case "compactKeepN":
		steps := []int{5, 10, 15, 20, 30, 50}
		cur := p.merged.GetCompactKeepN()
		next := steps[0]
		for i, v := range steps {
			if cur == v && i+1 < len(steps) {
				next = steps[i+1]
				break
			}
		}
		target.CompactKeepN = next
		newVal = fmt.Sprintf("%d", next)
	case "compactMode":
		modes := []string{"auto", "manual", "strategic"}
		target.CompactMode = cycleValue(p.merged.CompactMode, modes, "auto")
		newVal = target.CompactMode
	case "outputFilter":
		target.OutputFilter = !p.merged.OutputFilter
		newVal = fmt.Sprintf("%v", target.OutputFilter)
	case "outputStyle":
		modes := []string{"normal", "concise", "verbose", "markdown"}
		current := p.merged.OutputStyle
		if current == "" {
			current = "normal"
		}
		target.OutputStyle = cycleValue(current, modes, "normal")
		newVal = target.OutputStyle
	case "codeFilterLevel":
		levels := []string{"none", "minimal", "aggressive"}
		target.CodeFilterLevel = cycleValue(p.merged.CodeFilterLevel, levels, "minimal")
		newVal = target.CodeFilterLevel
	}

	p.saveProjectSetting(e.Key, newVal)
	p.reloadMerged()
	return e.Key, newVal
}

// ensureProjectConfig creates .claudio/ and initializes the project config
// if they don't exist yet, so that toggles always write to local config.
func (p *Panel) ensureProjectConfig() {
	if p.hasProject && p.project != nil {
		return
	}
	cwd, _ := os.Getwd()
	projectRoot := config.FindGitRoot(cwd)
	claudioDir := filepath.Join(projectRoot, ".claudio")
	os.MkdirAll(claudioDir, 0755)
	p.projectPath = filepath.Join(claudioDir, "settings.json")
	p.project = &config.Settings{}
	p.hasProject = true
	p.editScope = ScopeProject
}

// saveProjectSetting writes a single key to the project config file using
// raw JSON merge, which correctly handles bool false values (omitempty).
func (p *Panel) saveProjectSetting(key, value string) {
	dir := filepath.Dir(p.projectPath)
	os.MkdirAll(dir, 0755)

	var existing map[string]json.RawMessage
	if data, err := os.ReadFile(p.projectPath); err == nil {
		json.Unmarshal(data, &existing)
	}
	if existing == nil {
		existing = make(map[string]json.RawMessage)
	}

	// Marshal the full project settings to pick up struct field values
	data, _ := json.Marshal(p.project)
	var fresh map[string]json.RawMessage
	json.Unmarshal(data, &fresh)

	// Write the changed key. For bool fields, omitempty drops "false" from
	// the struct marshal, so we fall back to encoding the value directly.
	if raw, ok := fresh[key]; ok {
		existing[key] = raw
	} else if value == "true" || value == "false" {
		existing[key] = json.RawMessage(value)
	} else {
		valJSON, _ := json.Marshal(value)
		existing[key] = valJSON
	}

	out, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(p.projectPath, out, 0644)
}

// reloadMerged re-reads and merges configs after a change.
func (p *Panel) reloadMerged() {
	cwd, _ := os.Getwd()
	projectRoot := config.FindGitRoot(cwd)
	merged, _ := config.Load(projectRoot)
	if merged != nil {
		*p.merged = *merged
	}
}

// savePermissionRules saves the current permission rules to the appropriate config file.
func (p *Panel) savePermissionRules() {
	savePath := p.globalPath
	if p.hasProject && p.projectPath != "" {
		savePath = p.projectPath
	}

	data, _ := os.ReadFile(savePath)
	var existing map[string]json.RawMessage
	if json.Unmarshal(data, &existing) != nil {
		existing = make(map[string]json.RawMessage)
	}

	rulesJSON, _ := json.Marshal(p.merged.PermissionRules)
	existing["permissionRules"] = rulesJSON

	out, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(savePath, out, 0644)
}




func (p *Panel) View() string {
	if !p.active {
		return ""
	}

	var b strings.Builder

	// Title with scope indicator
	title := styles.PanelTitle.Render("Configuration")
	b.WriteString(title)
	b.WriteString("\n")

	// Scope tabs (pill style)
	if p.hasProject {
		var pill1, pill2 string
		if p.editScope == ScopeProject {
			pill1 = cfgActivePill.Render("● project")
			pill2 = cfgInactivePill.Render("  ○ global")
		} else {
			pill1 = cfgInactivePill.Render("○ project")
			pill2 = cfgActivePill.Render("  ● global")
		}
		tabHint := cfgTabHint.Render("  tab to switch")
		b.WriteString(pill1 + pill2 + tabHint)
		b.WriteString("\n")
	} else {
		b.WriteString(cfgNoProject.Render("  global only (run claudio init for project config)"))
		b.WriteString("\n")
	}

	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")



	listH := p.height - 6
	if listH < 3 {
		listH = 3
	}
	startIdx := 0
	if p.cursor >= listH {
		startIdx = p.cursor - listH + 1
	}
	endIdx := startIdx + listH
	if endIdx > len(p.entries) {
		endIdx = len(p.entries)
	}



	for i := startIdx; i < endIdx; i++ {
		e := p.entries[i]
		selected := i == p.cursor

		prefix := "  "
		if selected {
			prefix = styles.ViewportCursor.Render("▸ ")
		}

		// Providers section header
		if e.Key == "providers" {
			b.WriteString("\n")
			header := cfgSectionHeader.Render("── Providers ──")
			count := cfgValDim.Render(" " + e.Value)
			b.WriteString(prefix + header + count)
			b.WriteString("\n")
			continue
		}

		// Permission rules section header
		if e.Key == "permissions" {
			b.WriteString("\n")
			header := cfgSectionHeader.Render("── Permission Rules ──")
			count := cfgValDim.Render(" " + e.Value)
			b.WriteString(prefix + header + count)
			b.WriteString("\n")
			continue
		}

		// Permission rule entries
		if e.RuleIndex >= 0 {
			var behaviorTag string
			if e.EditType == "allow" {
				behaviorTag = cfgAllowBadge.Render(" allow ")
			} else {
				behaviorTag = cfgDenyBadge.Render(" " + e.EditType + " ")
			}
			tool := cfgRuleTool.Render(e.Key)
			pattern := cfgRulePattern.Render(e.Value)

			// Source badge
			var badge string
			if p.hasProject {
				if e.Source == ScopeProject {
					badge = " " + cfgProjBadge.Render(" P ")
				} else {
					badge = " " + cfgGlobalBadge.Render(" G ")
				}
			}

			line := prefix + behaviorTag + " " + tool + " " + pattern + badge
			b.WriteString("    " + line)
			b.WriteString("\n")
			continue
		}

		// Regular config entries
		key := cfgKeyStyle.Render(e.Key)
		var val string
		if isDefault(e.Value) {
			val = cfgValDim.Render(e.Value)
		} else {
			val = cfgValStyle.Render(e.Value)
		}

		// Source badge
		var badge string
		if p.hasProject {
			if e.Source == ScopeProject {
				badge = " " + cfgProjBadge.Render(" P ")
			} else {
				badge = " " + cfgGlobalBadge.Render(" G ")
			}
		}

		line := prefix + key + " " + val + badge
		if e.Editable && selected {
			line += " " + cfgEditHint.Render("⏎")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	hint := "  j/k navigate · enter toggle · d delete rule · esc close"
	if p.hasProject {
		scopeName := "project"
		if p.editScope == ScopeGlobal {
			scopeName = "global"
		}
		hint = fmt.Sprintf("  j/k · enter toggle (%s) · d delete rule · tab scope · esc", scopeName)
	}
	b.WriteString(styles.PanelHint.Render(hint))

	return b.String()
}

// cycleValue advances to the next value in the list, wrapping around.
func cycleValue(current string, options []string, fallback string) string {
	if current == "" {
		current = fallback
	}
	for i, opt := range options {
		if opt == current {
			return options[(i+1)%len(options)]
		}
	}
	return options[0]
}

func valOrDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// shortenPath trims an absolute path to a relative one from the git root.
func shortenPath(pattern string) string {
	if !filepath.IsAbs(pattern) {
		return pattern
	}
	cwd, _ := os.Getwd()
	root := config.FindGitRoot(cwd)
	if rel, err := filepath.Rel(root, pattern); err == nil {
		return rel
	}
	// Fall back to just the base name with parent hint
	dir := filepath.Base(filepath.Dir(pattern))
	return dir + "/" + filepath.Base(pattern)
}

func isDefault(v string) bool {
	switch v {
	case "claude-sonnet-4-6", "claude-haiku-4-5", "default", "auto",
		"standard", "false", "unlimited", "ai", "true", "normal",
		"(none)":
		return true
	}
	return false
}

// Help returns a short keybinding hint line for the panel footer.
func (p *Panel) Help() string {
	return "j/k navigate · enter toggle · d delete rule · tab scope · esc close"
}
