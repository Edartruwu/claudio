// Package config implements the configuration viewer/editor side panel.
// It supports editing both project-level (.claudio/settings.json) and
// global (~/.claudio/settings.json) settings, with visual distinction.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Scope identifies where a setting lives.
type Scope int

const (
	ScopeGlobal  Scope = iota // ~/.claudio/settings.json
	ScopeProject              // .claudio/settings.json
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
	addE("sessionPersist", fmt.Sprintf("%v", m.SessionPersist), "bool", p.source("sessionPersist"))
	addR("hookProfile", valOrDefault(m.HookProfile, "standard"), p.source("hookProfile"))

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
		for name, pc := range m.Providers {
			p.entries = append(p.entries, configEntry{
				Key: "  " + name, Value: fmt.Sprintf("%s (%s)", pc.APIBase, pc.Type),
				Source: p.source("providers"), RuleIndex: -1,
			})
			for shortcut, modelID := range pc.Models {
				p.entries = append(p.entries, configEntry{
					Key: "    /" + shortcut, Value: modelID,
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
	case "outputFilter":
		if p.project.OutputFilter {
			return ScopeProject
		}
	case "outputStyle":
		if p.project.OutputStyle != "" {
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
		if p.cursor < len(p.entries) && p.entries[p.cursor].Editable {
			key, val := p.toggleEntry(p.cursor)
			p.buildEntries()
			return func() tea.Msg {
				return ConfigChangedMsg{Key: key, Value: val}
			}, true
		}
		return nil, true
	}
	return nil, false
}

func (p *Panel) toggleEntry(idx int) (string, string) {
	e := p.entries[idx]
	target := p.targetSettings()
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
	}

	p.saveTarget()
	p.reloadMerged()
	return e.Key, newVal
}

// targetSettings returns the settings object for the current edit scope.
func (p *Panel) targetSettings() *config.Settings {
	if p.editScope == ScopeProject && p.project != nil {
		return p.project
	}
	return p.global
}

// saveTarget writes the current edit scope's settings to disk.
func (p *Panel) saveTarget() {
	if p.editScope == ScopeProject && p.projectPath != "" {
		saveSettingsFile(p.project, p.projectPath)
	} else {
		saveSettingsFile(p.global, p.globalPath)
	}
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

func saveSettingsFile(s *config.Settings, path string) {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0644)
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

	// Scope tabs
	if p.hasProject {
		projTab := "  project  "
		globalTab := "  global  "
		activeTab := lipgloss.NewStyle().Foreground(styles.Text).Bold(true).Underline(true)
		inactiveTab := lipgloss.NewStyle().Foreground(styles.Dim)

		if p.editScope == ScopeProject {
			b.WriteString(activeTab.Render(projTab))
			b.WriteString(inactiveTab.Render(globalTab))
		} else {
			b.WriteString(inactiveTab.Render(projTab))
			b.WriteString(activeTab.Render(globalTab))
		}
		b.WriteString("\n")
	} else {
		noProject := lipgloss.NewStyle().Foreground(styles.Dim).Italic(true)
		b.WriteString(noProject.Render("  global only (run claudio init for project config)"))
		b.WriteString("\n")
	}

	b.WriteString(styles.SeparatorLine(p.width))
	b.WriteString("\n")

	keyStyle := lipgloss.NewStyle().Foreground(styles.Aqua)
	valStyle := lipgloss.NewStyle().Foreground(styles.Text)
	valDimStyle := lipgloss.NewStyle().Foreground(styles.Dim)
	editHint := lipgloss.NewStyle().Foreground(styles.Warning)
	projBadge := lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Orange)
	globalBadge := lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Secondary)

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

	allowBadge := lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Success)
	denyBadge := lipgloss.NewStyle().Foreground(styles.Surface).Background(styles.Error)
	ruleToolStyle := lipgloss.NewStyle().Foreground(styles.Warning).Bold(true)
	rulePatternStyle := lipgloss.NewStyle().Foreground(styles.Dim)
	sectionHeader := lipgloss.NewStyle().Foreground(styles.Muted).Bold(true)

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
			header := sectionHeader.Render("── Providers ──")
			count := valDimStyle.Render(" " + e.Value)
			b.WriteString(prefix + header + count)
			b.WriteString("\n")
			continue
		}

		// Permission rules section header
		if e.Key == "permissions" {
			b.WriteString("\n")
			header := sectionHeader.Render("── Permission Rules ──")
			count := valDimStyle.Render(" " + e.Value)
			b.WriteString(prefix + header + count)
			b.WriteString("\n")
			continue
		}

		// Permission rule entries
		if e.RuleIndex >= 0 {
			var behaviorTag string
			if e.EditType == "allow" {
				behaviorTag = allowBadge.Render(" allow ")
			} else {
				behaviorTag = denyBadge.Render(" " + e.EditType + " ")
			}
			tool := ruleToolStyle.Render(e.Key)
			pattern := rulePatternStyle.Render(e.Value)

			// Source badge
			var badge string
			if p.hasProject {
				if e.Source == ScopeProject {
					badge = " " + projBadge.Render(" P ")
				} else {
					badge = " " + globalBadge.Render(" G ")
				}
			}

			line := prefix + behaviorTag + " " + tool + " " + pattern + badge
			b.WriteString("    " + line)
			b.WriteString("\n")
			continue
		}

		// Regular config entries
		key := keyStyle.Render(e.Key)
		var val string
		if isDefault(e.Value) {
			val = valDimStyle.Render(e.Value)
		} else {
			val = valStyle.Render(e.Value)
		}

		// Source badge
		var badge string
		if p.hasProject {
			if e.Source == ScopeProject {
				badge = " " + projBadge.Render(" P ")
			} else {
				badge = " " + globalBadge.Render(" G ")
			}
		}

		line := prefix + key + " " + val + badge
		if e.Editable && selected {
			line += " " + editHint.Render("⏎")
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

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Render(b.String())
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
