package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/teams"
)

// ─── SaveTeamTemplate ────────────────────────────────────────────────────────

// SaveTeamTemplateTool saves the current active team's member composition as a
// reusable template stored at ~/.claudio/team-templates/{name}.json.
type SaveTeamTemplateTool struct {
	deferrable
	Runner  *teams.TeammateRunner
	Manager *teams.Manager
}

func (t *SaveTeamTemplateTool) Name() string { return "SaveTeamTemplate" }

func (t *SaveTeamTemplateTool) Description() string {
	return `Save the current team's member composition as a reusable template.

The template captures each member's name and subagent_type so the same team
structure can be recreated later with InstantiateTeam.`
}

func (t *SaveTeamTemplateTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Template name (e.g. 'coding-team', 'research-team'). Used as the filename."
			}
		},
		"required": ["name"]
	}`)
}

func (t *SaveTeamTemplateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.Name == "" {
		return &Result{Content: "name is required", IsError: true}, nil
	}

	teamName := t.Runner.ActiveTeamName()
	if teamName == "" {
		return &Result{Content: "no active team — create or join a team first", IsError: true}, nil
	}

	tmpl, err := t.Manager.SaveAsTemplate(teamName, in.Name)
	if err != nil {
		return &Result{Content: fmt.Sprintf("failed to save template: %v", err), IsError: true}, nil
	}

	var lines []string
	for _, m := range tmpl.Members {
		line := fmt.Sprintf("  - %s (%s)", m.Name, m.SubagentType)
		if m.Model != "" {
			line += " model=" + m.Model
		}
		lines = append(lines, line)
	}
	return &Result{Content: fmt.Sprintf("Template %q saved with %d members:\n%s", in.Name, len(tmpl.Members), strings.Join(lines, "\n"))}, nil
}

func (t *SaveTeamTemplateTool) IsReadOnly() bool                        { return false }
func (t *SaveTeamTemplateTool) RequiresApproval(_ json.RawMessage) bool { return false }

// ─── InstantiateTeam ─────────────────────────────────────────────────────────

// InstantiateTeamTool creates a team from a saved template, pre-registering all
// members so the lead can assign work via SpawnTeammate.
type InstantiateTeamTool struct {
	deferrable
	Runner         *teams.TeammateRunner
	Manager        *teams.Manager
	GetSessionID   func() string // returns current session ID at execution time
	InstantiatedTeam string      // name of the team created by this tool (for cleanup on Close)
}

func (t *InstantiateTeamTool) Name() string { return "InstantiateTeam" }

func (t *InstantiateTeamTool) Description() string {
	return `Create a team from a saved template.

Loads the template by name, creates the team, and pre-registers all members
(with their subagent_type) so you know the roster upfront. Use SpawnTeammate
to assign actual work to each member.

IMPORTANT: Always provide a unique team_name scoped to the current project
(e.g. "coding-team-myproject"). The same template may be reused across
multiple projects simultaneously — if you omit team_name, it defaults to the
template name and will conflict with any other session using the same template.
A good convention is "{template_name}-{project_slug}".`
}

func (t *InstantiateTeamTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"template_name": {
				"type": "string",
				"description": "Name of the template to load (e.g. 'coding-team')."
			},
			"team_name": {
				"type": "string",
				"description": "Project-scoped team name. REQUIRED in practice — never omit this. Use the format '{template_name}-{project_slug}' (e.g. 'backend-team-payments'). Omitting it defaults to the bare template name, which will conflict with any other session using the same template simultaneously."
			}
		},
		"required": ["template_name"]
	}`)
}

func (t *InstantiateTeamTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in struct {
		TemplateName string `json:"template_name"`
		TeamName     string `json:"team_name"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.TemplateName == "" {
		return &Result{Content: "template_name is required", IsError: true}, nil
	}

	tmpl, err := t.Manager.GetTemplate(in.TemplateName)
	if err != nil {
		return &Result{Content: fmt.Sprintf("template not found: %v", err), IsError: true}, nil
	}

	// Resolve session ID via closure if available.
	sessionID := ""
	if t.GetSessionID != nil {
		sessionID = t.GetSessionID()
	}

	teamName := in.TeamName
	if teamName == "" {
		if sessionID != "" && len(sessionID) >= 8 {
			teamName = tmpl.Name + "-" + sessionID[:8]
		} else {
			teamName = tmpl.Name
		}
	}

	if _, err := t.Manager.CreateTeam(teamName, tmpl.Description, sessionID, tmpl.Model); err != nil {
		// Team may already exist; proceed anyway
		_ = err
	}
	t.InstantiatedTeam = teamName
	t.Runner.SetActiveTeam(teamName)
	if tmpl.AutoCompactThreshold > 0 {
		t.Manager.SetAutoCompactThreshold(teamName, tmpl.AutoCompactThreshold)
	}

	// Pre-register members so their subagent_type is persisted before work is assigned
	var roster []string
	for _, m := range tmpl.Members {
		model := m.Model
		if model == "" {
			model = tmpl.Model
		}
		_, _ = t.Manager.AddMember(teamName, m.Name, model, "", m.SubagentType, m.AutoCompactThreshold)
		line := fmt.Sprintf("  - %s (%s)", m.Name, m.SubagentType)
		if model != "" {
			line += " model=" + model
		}
		roster = append(roster, line)
	}

	msg := fmt.Sprintf("Team %q instantiated from template %q with %d members:\n%s\n\nUse SpawnTeammate to assign tasks to each member.",
		teamName, tmpl.Name, len(tmpl.Members), strings.Join(roster, "\n"))
	return &Result{Content: msg}, nil
}

func (t *InstantiateTeamTool) IsReadOnly() bool                        { return false }
func (t *InstantiateTeamTool) RequiresApproval(_ json.RawMessage) bool { return false }
