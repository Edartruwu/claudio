package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/teams"
)

type InstantiateTeamTool struct {
	deferrable
	Runner           *teams.TeammateRunner
	Manager          *teams.Manager
	GetSessionID     func() string // returns current session ID at execution time
	InstantiatedTeam string        // name of the team created by this tool (for cleanup on Close)
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
		_ = err
	}
	t.InstantiatedTeam = teamName
	t.Runner.SetActiveTeam(teamName)
	if tmpl.AutoCompactThreshold > 0 {
		t.Manager.SetAutoCompactThreshold(teamName, tmpl.AutoCompactThreshold)
	}

	var roster []string
	for _, m := range tmpl.Members {
		model := m.Model
		if model == "" {
			model = tmpl.Model
		}
		_, _ = t.Manager.AddMember(teamName, m.Name, model, "", m.SubagentType, m.AutoCompactThreshold)
		if m.Advisor != nil {
			t.Manager.SetMemberAdvisorConfig(teamName, m.Name, m.Advisor)
		}
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
