package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/services/memory"
)

// CrystallizeSession creates a new agent definition from a session's accumulated knowledge.
// It writes an agent markdown file and copies relevant memories into the agent's own memory directory.
func CrystallizeSession(agentsDir, name, description, sessionID, sourceProject, summary string, memories []*memory.Entry) (*AgentDefinition, error) {
	if name == "" {
		return nil, fmt.Errorf("agent name required")
	}

	// Sanitize name for filesystem
	safeName := sanitizeAgentName(name)

	// Create agent definition file
	agentFile := filepath.Join(agentsDir, safeName+".md")
	memDir := filepath.Join(agentsDir, safeName, "memory")

	if err := os.MkdirAll(memDir, 0755); err != nil {
		return nil, fmt.Errorf("creating agent memory dir: %w", err)
	}

	// Build system prompt from summary and key memories
	systemPrompt := buildAgentPrompt(name, description, summary, memories)

	// Build frontmatter
	var content strings.Builder
	content.WriteString("---\n")
	content.WriteString(fmt.Sprintf("description: %s\n", description))
	if sessionID != "" {
		content.WriteString(fmt.Sprintf("sourceSession: %s\n", sessionID))
	}
	if sourceProject != "" {
		content.WriteString(fmt.Sprintf("sourceProject: %s\n", sourceProject))
	}
	content.WriteString("tools: \"*\"\n")
	content.WriteString("---\n\n")
	content.WriteString(systemPrompt)

	if err := os.WriteFile(agentFile, []byte(content.String()), 0644); err != nil {
		return nil, fmt.Errorf("writing agent file: %w", err)
	}

	// Copy memories into agent's memory directory
	agentMemStore := memory.NewStore(memDir)
	for _, m := range memories {
		entry := &memory.Entry{
			Name:        m.Name,
			Description: m.Description,
			Type:        m.Type,
			Content:     m.Content,
		}
		if err := agentMemStore.Save(entry); err != nil {
			// Non-fatal: continue copying other memories
			continue
		}
	}

	return &AgentDefinition{
		Type:          safeName,
		WhenToUse:     description,
		SystemPrompt:  systemPrompt,
		Tools:         []string{"*"},
		MemoryDir:     memDir,
		SourceSession: sessionID,
		SourceProject: sourceProject,
	}, nil
}

func buildAgentPrompt(name, description, summary string, memories []*memory.Entry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are %s, a specialized agent.\n\n", name))

	if description != "" {
		sb.WriteString(fmt.Sprintf("## Purpose\n%s\n\n", description))
	}

	if summary != "" {
		sb.WriteString(fmt.Sprintf("## Background\n%s\n\n", summary))
	}

	sb.WriteString(`## Guidelines
- You have access to all tools including file operations, search, and shell commands.
- Your memory directory contains knowledge from your previous sessions.
- Apply the patterns and preferences you've learned.
- Report your findings clearly and concisely when done.
`)

	if len(memories) > 0 {
		sb.WriteString("\n## Key Knowledge\n")
		for _, m := range memories {
			if len(sb.String()) > 8000 {
				sb.WriteString("\n... (additional knowledge available in memory files)\n")
				break
			}
			sb.WriteString(fmt.Sprintf("\n### %s (%s)\n%s\n", m.Name, m.Type, m.Content))
		}
	}

	return sb.String()
}

func sanitizeAgentName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	// Keep only alphanumeric, hyphens, underscores
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, name)
	if len(safe) > 50 {
		safe = safe[:50]
	}
	return safe
}
