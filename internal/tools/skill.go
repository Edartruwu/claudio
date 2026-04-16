package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	doublestar "github.com/bmatcuk/doublestar/v4"

	"github.com/Abraxas-365/claudio/internal/hooks"
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
	HooksManager   *hooks.Manager // nil-safe — skip if not wired
	ProjectRoot    string         // for paths: filtering; empty = no filtering

	registeredHooks   map[string]bool
	registeredHooksMu sync.Mutex

	// cachedDescription is built once on first Description() call.
	// Keeps the tool description byte-stable across turns so the Anthropic
	// prompt cache is not busted every request.
	cachedDescription     string
	cachedDescriptionOnce sync.Once
}

// skillMatchesPaths returns true if any file under projectRoot matches any pattern.
// Returns true if patterns is empty or projectRoot is unset.
func skillMatchesPaths(projectRoot string, patterns []string) bool {
	if len(patterns) == 0 || projectRoot == "" {
		return true
	}
	fsys := os.DirFS(projectRoot)
	for _, pattern := range patterns {
		matches, _ := doublestar.Glob(fsys, pattern)
		if len(matches) > 0 {
			return true
		}
	}
	return false
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

	// Register skill hooks on first invocation (idempotent).
	if t.HooksManager != nil && len(skill.Hooks) > 0 {
		t.registeredHooksMu.Lock()
		if t.registeredHooks == nil {
			t.registeredHooks = make(map[string]bool)
		}
		if !t.registeredHooks[skill.Name] {
			for _, h := range skill.Hooks {
				defs := []hooks.HookDef{{
					Type:    "command",
					Command: h.Command,
					Timeout: h.Timeout,
					Async:   h.Async,
				}}
				t.HooksManager.RegisterSkillHooks(h.Event, h.Matcher, defs)
			}
			t.registeredHooks[skill.Name] = true
		}
		t.registeredHooksMu.Unlock()
	}

	content := skill.Content
	if skill.SkillDir != "" {
		content = "Base directory for this skill: " + skill.SkillDir + "\n\n" + content
		content = strings.ReplaceAll(content, "${CLAUDE_SKILL_DIR}", skill.SkillDir)
		dirName := filepath.Base(skill.SkillDir)
		content = strings.ReplaceAll(content, ".claude/skills/"+dirName+"/", skill.SkillDir+"/")
		content = strings.ReplaceAll(content, ".claudio/skills/"+dirName+"/", skill.SkillDir+"/")
		content = strings.ReplaceAll(content, "skills/"+dirName+"/", skill.SkillDir+"/")
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
		if !skillMatchesPaths(t.ProjectRoot, s.Paths) {
			continue
		}
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
