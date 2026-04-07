package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// editorFinishedMsg is sent when the external editor process exits.
type editorFinishedMsg struct {
	content string
	err     error
}

// planEditorFinishedMsg is sent when the plan file editor exits (from the plan approval dialog).
type planEditorFinishedMsg struct {
	err error
}



// getEditor returns the user's preferred editor from $VISUAL or $EDITOR,
// falling back to "vi".
func getEditor() string {
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

// openExternalEditor writes content to a temp file, opens the user's editor,
// and returns the edited content via editorFinishedMsg.
func openExternalEditor(content string) tea.Cmd {
	tmpFile, err := os.CreateTemp("", "claudio-prompt-*.md")
	if err != nil {
		return func() tea.Msg {
			return editorFinishedMsg{err: err}
		}
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return func() tea.Msg {
			return editorFinishedMsg{err: err}
		}
	}
	tmpFile.Close()

	editor := getEditor()
	parts := strings.Fields(editor)
	name := parts[0]
	args := append(parts[1:], tmpPath)

	c := exec.Command(name, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return editorFinishedMsg{err: err}
		}
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return editorFinishedMsg{err: readErr}
		}
		return editorFinishedMsg{content: strings.TrimRight(string(data), "\n")}
	})
}

// askUserEditorFinishedMsg is sent when the external editor exits from the AskUser "Other" input.
type askUserEditorFinishedMsg struct {
	content string
	err     error
}

// openAskUserEditor writes content to a temp file, opens the user's editor,
// and returns the edited content via askUserEditorFinishedMsg.
func openAskUserEditor(content string) tea.Cmd {
	tmpFile, err := os.CreateTemp("", "claudio-askuser-*.md")
	if err != nil {
		return func() tea.Msg { return askUserEditorFinishedMsg{err: err} }
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return func() tea.Msg { return askUserEditorFinishedMsg{err: err} }
	}
	tmpFile.Close()
	editor := getEditor()
	parts := strings.Fields(editor)
	name := parts[0]
	args := append(parts[1:], tmpPath)
	c := exec.Command(name, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return askUserEditorFinishedMsg{err: err}
		}
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return askUserEditorFinishedMsg{err: readErr}
		}
		return askUserEditorFinishedMsg{content: strings.TrimRight(string(data), "\n")}
	})
}

// fileEditorFinishedMsg is sent after openFileInEditor returns.
type fileEditorFinishedMsg struct {
	path string
	err  error
}

// openFileInEditor opens an existing file in the user's editor.
// editorCmd may be a template like "nvim -c 'Gvdiffsplit ORIG_HEAD' {file}";
// {file} is replaced with path. If empty, falls back to $VISUAL/$EDITOR.
func openFileInEditor(path, editorCmd string) tea.Cmd {
	var name string
	var args []string

	if editorCmd != "" {
		expanded := strings.ReplaceAll(editorCmd, "{file}", path)
		parts := shellSplit(expanded)
		name = parts[0]
		args = parts[1:]
	} else {
		editor := getEditor()
		parts := strings.Fields(editor)
		name = parts[0]
		args = append(parts[1:], path)
	}

	c := exec.Command(name, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return fileEditorFinishedMsg{path: path, err: err}
	})
}

// shellSplit splits s into tokens respecting single and double quoted strings.
func shellSplit(s string) []string {
	var tokens []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(c)
			}
		case inDouble:
			if c == '"' {
				inDouble = false
			} else {
				cur.WriteByte(c)
			}
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == ' ' || c == '\t':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// openPlanEditor opens the plan file directly in the user's editor.
// When the editor exits, it sends planEditorFinishedMsg so the caller
// can restore the plan approval dialog.
func openPlanEditor(path string) tea.Cmd {
	editor := getEditor()
	parts := strings.Fields(editor)
	name := parts[0]
	args := append(parts[1:], path)

	c := exec.Command(name, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return planEditorFinishedMsg{err: err}
	})
}
