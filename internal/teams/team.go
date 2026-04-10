// Package teams provides multi-agent team coordination.
// Teams allow spawning multiple AI agents that work in parallel,
// communicate via a file-based mailbox, and share a task list.
package teams

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TeamConfig is the on-disk team configuration stored at ~/.claudio/teams/{name}/config.json.
type TeamConfig struct {
	Name                 string        `json:"name"`
	Description          string        `json:"description,omitempty"`
	Model                string        `json:"model,omitempty"` // default model for all agents in this team
	AutoCompactThreshold int           `json:"autoCompactThreshold,omitempty"` // % context to trigger compact (team-level default)
	LeadAgent            string        `json:"lead_agent"`      // e.g., "team-lead@my-team"
	LeadSession          string        `json:"lead_session"`    // leader's session ID
	Members              []*TeamMember `json:"members"`
	CreatedAt            time.Time     `json:"created_at"`
	AllowPaths           []string      `json:"allow_paths,omitempty"` // shared filesystem access
}

// TeamMember describes a teammate.
type TeamMember struct {
	Identity             TeammateIdentity `json:"identity"`
	Status               MemberStatus     `json:"status"`
	JoinedAt             time.Time        `json:"joined_at"`
	TaskID               string           `json:"task_id,omitempty"`              // background task ID
	Model                string           `json:"model,omitempty"`
	Prompt               string           `json:"prompt,omitempty"`
	SubagentType         string           `json:"subagent_type,omitempty"`        // agent definition used (e.g. "backend-senior")
	AutoCompactThreshold int              `json:"autoCompactThreshold,omitempty"` // % context to trigger compact (overrides team-level)
	AdvisorConfig        *AdvisorConfig   `json:"advisor,omitempty"`              // optional advisor config (injected at spawn)
	SystemPrompt         string           `json:"system_prompt,omitempty"`        // resolved system prompt (for revival after eviction)
}

// TeammateIdentity uniquely identifies an agent within a team.
type TeammateIdentity struct {
	AgentID   string `json:"agent_id"`   // "researcher@my-team"
	AgentName string `json:"agent_name"` // "researcher"
	TeamName  string `json:"team_name"`  // "my-team"
	Color     string `json:"color"`      // TUI display color
	IsLead    bool   `json:"is_lead,omitempty"`
}

// MemberStatus tracks a teammate's lifecycle.
type MemberStatus string

const (
	StatusIdle             MemberStatus = "idle"
	StatusWorking          MemberStatus = "working"
	StatusComplete         MemberStatus = "complete"
	StatusFailed           MemberStatus = "failed"
	StatusShutdown         MemberStatus = "shutdown"
	StatusWaitingForInput  MemberStatus = "waiting_for_input"
)

// Available colors for teammates (gruvbox palette).
var teamColors = []string{
	"#b8bb26", // green
	"#83a598", // blue
	"#d3869b", // purple
	"#fabd2f", // yellow
	"#fe8019", // orange
	"#8ec07c", // aqua
	"#fb4934", // red
	"#d65d0e", // dark orange
}

// FormatAgentID creates a deterministic agent ID from name and team.
func FormatAgentID(name, teamName string) string {
	return fmt.Sprintf("%s@%s", strings.ToLower(name), strings.ToLower(teamName))
}

// Manager handles team lifecycle and coordination.
type Manager struct {
	mu           sync.RWMutex
	teamsDir     string
	templatesDir string
	active       map[string]*TeamConfig // keyed by team name
}

// NewManager creates a team manager.
func NewManager(teamsDir, templatesDir string) *Manager {
	os.MkdirAll(teamsDir, 0700)
	if templatesDir != "" {
		os.MkdirAll(templatesDir, 0700)
	}
	m := &Manager{
		teamsDir:     teamsDir,
		templatesDir: templatesDir,
		active:       make(map[string]*TeamConfig),
	}
	m.loadActive()
	return m
}

// SaveAsTemplate saves a team's non-lead members as a reusable template.
func (m *Manager) SaveAsTemplate(teamName, templateName string) (*TeamTemplate, error) {
	m.mu.RLock()
	team, ok := m.active[teamName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("team %q not found", teamName)
	}

	var members []TeamTemplateMember
	for _, mem := range team.Members {
		if mem.Identity.IsLead {
			continue
		}
		members = append(members, TeamTemplateMember{
			Name:         mem.Identity.AgentName,
			SubagentType: mem.SubagentType,
			Model:        mem.Model,
		})
	}

	t := TeamTemplate{
		Name:        templateName,
		Description: team.Description,
		Model:       team.Model,
		Members:     members,
	}
	if err := SaveTemplate(m.templatesDir, t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTemplates returns all saved team templates.
func (m *Manager) ListTemplates() []TeamTemplate {
	return LoadTemplates(m.templatesDir)
}

// GetTemplate returns a single template by name.
func (m *Manager) GetTemplate(name string) (*TeamTemplate, error) {
	return GetTemplate(m.templatesDir, name)
}

// CreateTeam initializes a new team with the calling agent as lead.
func (m *Manager) CreateTeam(name, description, sessionID, model string) (*TeamConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.active[name]; exists {
		return nil, fmt.Errorf("team %q already exists", name)
	}

	leadID := FormatAgentID("team-lead", name)

	team := &TeamConfig{
		Name:        name,
		Description: description,
		Model:       model,
		LeadAgent:   leadID,
		LeadSession: sessionID,
		CreatedAt:   time.Now(),
		Members: []*TeamMember{
			{
				Identity: TeammateIdentity{
					AgentID:   leadID,
					AgentName: "team-lead",
					TeamName:  name,
					Color:     teamColors[0],
					IsLead:    true,
				},
				Status:   StatusWorking,
				JoinedAt: time.Now(),
			},
		},
	}

	// Create directories
	teamDir := filepath.Join(m.teamsDir, name)
	os.MkdirAll(filepath.Join(teamDir, "inboxes"), 0700)

	// Save config
	if err := m.saveConfig(team); err != nil {
		return nil, err
	}

	m.active[name] = team
	return team, nil
}

// SetAutoCompactThreshold sets the team-level auto-compact threshold (percentage).
func (m *Manager) SetAutoCompactThreshold(teamName string, threshold int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if team, ok := m.active[teamName]; ok {
		team.AutoCompactThreshold = threshold
		m.saveConfig(team)
	}
}

// SetMemberAdvisorConfig stores an AdvisorConfig on an existing team member.
// Called by InstantiateTeam after AddMember to persist the advisor spec.
func (m *Manager) SetMemberAdvisorConfig(teamName, agentName string, cfg *AdvisorConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	team, ok := m.active[teamName]
	if !ok {
		return
	}
	agentID := FormatAgentID(agentName, teamName)
	for _, mem := range team.Members {
		if mem.Identity.AgentID == agentID {
			mem.AdvisorConfig = cfg
			m.saveConfig(team)
			return
		}
	}
}

// AddMember adds a teammate to an existing team.
func (m *Manager) AddMember(teamName, agentName, model, prompt, subagentType string, autoCompactThreshold ...int) (*TeamMember, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.active[teamName]
	if !ok {
		return nil, fmt.Errorf("team %q not found", teamName)
	}

	// Check for duplicate — allow re-adding if previous member is in a terminal state
	agentID := FormatAgentID(agentName, teamName)
	for i, mem := range team.Members {
		if mem.Identity.AgentID == agentID {
			if mem.Status == StatusWorking {
				return nil, fmt.Errorf("member %q is still active in team", agentName)
			}
			// Terminal state (complete/failed/shutdown/waiting_for_input) — replace the old entry
			team.Members = append(team.Members[:i], team.Members[i+1:]...)
			break
		}
	}

	colorIdx := len(team.Members) % len(teamColors)

	member := &TeamMember{
		Identity: TeammateIdentity{
			AgentID:   agentID,
			AgentName: agentName,
			TeamName:  teamName,
			Color:     teamColors[colorIdx],
		},
		Status:   StatusIdle,
		JoinedAt: time.Now(),
		Model:                model,
		Prompt:               prompt,
		SubagentType:         subagentType,
		AutoCompactThreshold: func() int { if len(autoCompactThreshold) > 0 { return autoCompactThreshold[0] }; return 0 }(),
	}

	team.Members = append(team.Members, member)
	m.saveConfig(team)

	return member, nil
}

// UpdateMemberStatus changes a member's status.
func (m *Manager) UpdateMemberStatus(teamName, agentID string, status MemberStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.active[teamName]
	if !ok {
		return
	}

	for _, mem := range team.Members {
		if mem.Identity.AgentID == agentID {
			mem.Status = status
			break
		}
	}
	m.saveConfig(team)
}

// UpdateMemberSystemPrompt persists a member's resolved system prompt.
func (m *Manager) UpdateMemberSystemPrompt(teamName, agentID, systemPrompt string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.active[teamName]
	if !ok {
		return
	}

	for _, mem := range team.Members {
		if mem.Identity.AgentID == agentID {
			mem.SystemPrompt = systemPrompt
			break
		}
	}
	m.saveConfig(team)
}

// GetTeam returns a team by name.
func (m *Manager) GetTeam(name string) (*TeamConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	team, ok := m.active[name]
	return team, ok
}

// ListTeams returns all active teams.
func (m *Manager) ListTeams() []*TeamConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*TeamConfig, 0, len(m.active))
	for _, t := range m.active {
		result = append(result, t)
	}
	return result
}

// RemoveMember removes a member from the team config. Safe to call on inactive agents.
func (m *Manager) RemoveMember(teamName, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	team, ok := m.active[teamName]
	if !ok {
		return nil // team not found — no-op
	}
	newMembers := make([]*TeamMember, 0, len(team.Members))
	for _, mem := range team.Members {
		if mem.Identity.AgentID != agentID {
			newMembers = append(newMembers, mem)
		}
	}
	team.Members = newMembers
	return m.saveConfig(team)
}

// DeleteTeam removes a team and cleans up.
func (m *Manager) DeleteTeam(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.active[name]
	if !ok {
		return fmt.Errorf("team %q not found", name)
	}

	// Check for active members
	for _, mem := range team.Members {
		if mem.Status == StatusWorking && !mem.Identity.IsLead {
			return fmt.Errorf("cannot delete: member %q is still active", mem.Identity.AgentName)
		}
	}

	// Remove directory
	teamDir := filepath.Join(m.teamsDir, name)
	os.RemoveAll(teamDir)

	delete(m.active, name)
	return nil
}

// GetMember returns a member by agent ID.
func (m *Manager) GetMember(teamName, agentID string) (*TeamMember, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	team, ok := m.active[teamName]
	if !ok {
		return nil, false
	}
	for _, mem := range team.Members {
		if mem.Identity.AgentID == agentID {
			return mem, true
		}
	}
	return nil, false
}

// ActiveMembers returns non-lead members that are working.
func (m *Manager) ActiveMembers(teamName string) []*TeamMember {
	m.mu.RLock()
	defer m.mu.RUnlock()

	team, ok := m.active[teamName]
	if !ok {
		return nil
	}

	var active []*TeamMember
	for _, mem := range team.Members {
		if !mem.Identity.IsLead && mem.Status == StatusWorking {
			active = append(active, mem)
		}
	}
	return active
}

// AllMembers returns all non-lead members.
func (m *Manager) AllMembers(teamName string) []*TeamMember {
	m.mu.RLock()
	defer m.mu.RUnlock()

	team, ok := m.active[teamName]
	if !ok {
		return nil
	}

	var members []*TeamMember
	for _, mem := range team.Members {
		if !mem.Identity.IsLead {
			members = append(members, mem)
		}
	}
	return members
}

// TeamsDir returns the base teams directory.
func (m *Manager) TeamsDir() string {
	return m.teamsDir
}

// FormatTeamStatus returns a human-readable team summary.
func (m *Manager) FormatTeamStatus(teamName string) string {
	team, ok := m.GetTeam(teamName)
	if !ok {
		return fmt.Sprintf("Team %q not found", teamName)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Team: %s", team.Name))
	if team.Description != "" {
		sb.WriteString(fmt.Sprintf(" — %s", team.Description))
	}
	sb.WriteString(fmt.Sprintf("\nCreated: %s\n", team.CreatedAt.Format("15:04:05")))
	sb.WriteString(fmt.Sprintf("Members (%d):\n", len(team.Members)))

	for _, mem := range team.Members {
		icon := "○"
		switch mem.Status {
		case StatusWorking:
			icon = "◐"
		case StatusComplete:
			icon = "●"
		case StatusFailed:
			icon = "✗"
		case StatusShutdown:
			icon = "⊘"
		case StatusWaitingForInput:
			icon = "?"
		}
		role := ""
		if mem.Identity.IsLead {
			role = " (lead)"
		}
		sb.WriteString(fmt.Sprintf("  %s %s [%s]%s\n", icon, mem.Identity.AgentName, mem.Status, role))
	}

	return sb.String()
}

func (m *Manager) saveConfig(team *TeamConfig) error {
	teamDir := filepath.Join(m.teamsDir, team.Name)
	os.MkdirAll(teamDir, 0700)

	path := filepath.Join(teamDir, "config.json")
	data, err := json.MarshalIndent(team, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (m *Manager) loadActive() {
	entries, err := os.ReadDir(m.teamsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		configPath := filepath.Join(m.teamsDir, entry.Name(), "config.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}
		var team TeamConfig
		if json.Unmarshal(data, &team) == nil {
			m.active[team.Name] = &team
		}
	}
}
