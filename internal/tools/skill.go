package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/services/skills"
)

// shellInterpolRe matches !`cmd` patterns in skill content.
var shellInterpolRe = regexp.MustCompile("!`([^`]+)`")

// interpolateShellCommands replaces !`cmd` placeholders with live command output,
// mirroring Claude Code's executeShellCommandsInPrompt pattern. This lets skill
// content inject live context (e.g. git status/diff) at invocation time instead
// of asking the model to make extra tool calls to gather it.
func interpolateShellCommands(content string) string {
	return shellInterpolRe.ReplaceAllStringFunc(content, func(match string) string {
		cmd := shellInterpolRe.FindStringSubmatch(match)[1]
		out, err := exec.Command("sh", "-c", cmd).Output()
		if err != nil {
			return fmt.Sprintf("(error running `%s`: %v)", cmd, err)
		}
		return strings.TrimRight(string(out), "\n")
	})
}

// SkillTool allows agents to invoke skills by name, enabling auto-detection and
// proactive use of project-specific and domain-specific skill instructions.
type SkillTool struct {
	SkillsRegistry *skills.Registry

	// cachedDescription is built once on first Description() call.
	// Keeps the tool description byte-stable across turns so the Anthropic
	// prompt cache is not busted every request.
	cachedDescription     string
	cachedDescriptionOnce sync.Once
}

type skillInput struct {
	Skill     string `json:"skill"`
	Arguments string `json:"arguments,omitempty"`
}

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Description() string {
	t.cachedDescriptionOnce.Do(func() {
		t.cachedDescription = prompts.SkillDescription(t.formatSkillList())
	})
	return t.cachedDescription
}

func (t *SkillTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"skill": {
				"type": "string",
				"description": "Name of the skill to invoke (e.g. \"commit\", \"htmx\", \"review\")"
			},
			"arguments": {
				"type": "string",
				"description": "Optional arguments passed to the skill — replaces the $ARGUMENTS placeholder in the skill content"
			}
		},
		"required": ["skill"]
	}`)
}

func (t *SkillTool) IsReadOnly() bool                        { return true }
func (t *SkillTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *SkillTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in skillInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if t.SkillsRegistry == nil {
		return &Result{Content: "Skills not available (registry not initialised)", IsError: true}, nil
	}

	skill := t.findSkill(in.Skill)
	if skill == nil {
		return &Result{
			Content: fmt.Sprintf("Skill %q not found. Available skills: %s", in.Skill, t.availableNames()),
			IsError: true,
		}, nil
	}

	content := skill.Content
	if skill.SkillDir != "" {
		content = "Base directory for this skill: " + skill.SkillDir + "\n\n" + content
		content = strings.ReplaceAll(content, "${CLAUDE_SKILL_DIR}", skill.SkillDir)
	}
	if in.Arguments != "" {
		content = strings.ReplaceAll(content, "$ARGUMENTS", in.Arguments)
	}
	// Resolve !`cmd` placeholders before injection so the model immediately
	// has live context (e.g. git status/diff) without extra tool-call round-trips.
	content = interpolateShellCommands(content)

	// Inject skill content as a conversation message so it persists in history
	// and survives compaction — mirrors claude-code's newMessages mechanism.
	return &Result{
		Content:          fmt.Sprintf("Skill %q loaded. Follow the instructions it contains.", skill.Name),
		InjectedMessages: []string{content},
	}, nil
}

// findSkill looks up a skill by exact name, then falls back to case-insensitive match.
func (t *SkillTool) findSkill(name string) *skills.Skill {
	if s, ok := t.SkillsRegistry.Get(name); ok {
		return s
	}
	for _, s := range t.SkillsRegistry.All() {
		if strings.EqualFold(s.Name, name) {
			return s
		}
	}
	return nil
}

func (t *SkillTool) formatSkillList() string {
	if t.SkillsRegistry == nil {
		return "(no skills loaded)"
	}
	all := t.SkillsRegistry.All()
	if len(all) == 0 {
		return "(no skills available)"
	}
	var lines []string
	for _, s := range all {
		line := "- " + s.Name
		if s.Description != "" {
			line += ": " + s.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (t *SkillTool) availableNames() string {
	if t.SkillsRegistry == nil {
		return "(none)"
	}
	var names []string
	for _, s := range t.SkillsRegistry.All() {
		names = append(names, s.Name)
	}
	return strings.Join(names, ", ")
}
