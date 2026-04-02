package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Abraxas-365/claudio/internal/prompts"
)

// NotebookEditTool edits Jupyter notebook cells.
type NotebookEditTool struct {
	deferrable
}

type notebookInput struct {
	FilePath    string `json:"file_path"`
	CellIndex   int    `json:"cell_index"`
	NewContents string `json:"new_contents,omitempty"`
	Action      string `json:"action"` // "insert", "delete", "update"
}

func (t *NotebookEditTool) Name() string { return "NotebookEdit" }
func (t *NotebookEditTool) Description() string {
	return prompts.NotebookEditDescription()
}
func (t *NotebookEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {"type": "string", "description": "Path to the .ipynb file"},
			"cell_index": {"type": "number", "description": "Index of the cell to modify"},
			"new_contents": {"type": "string", "description": "New cell contents (for insert/update)"},
			"action": {"type": "string", "enum": ["insert", "delete", "update"], "description": "Action to perform"}
		},
		"required": ["file_path", "cell_index", "action"]
	}`)
}
func (t *NotebookEditTool) IsReadOnly() bool                        { return false }
func (t *NotebookEditTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *NotebookEditTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in notebookInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	// Read notebook
	data, err := os.ReadFile(in.FilePath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to read notebook: %v", err), IsError: true}, nil
	}

	var notebook map[string]interface{}
	if err := json.Unmarshal(data, &notebook); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid notebook format: %v", err), IsError: true}, nil
	}

	cells, ok := notebook["cells"].([]interface{})
	if !ok {
		return &Result{Content: "No cells found in notebook", IsError: true}, nil
	}

	switch in.Action {
	case "insert":
		newCell := map[string]interface{}{
			"cell_type": "code",
			"source":    []string{in.NewContents},
			"metadata":  map[string]interface{}{},
			"outputs":   []interface{}{},
		}
		if in.CellIndex >= len(cells) {
			cells = append(cells, newCell)
		} else {
			cells = append(cells[:in.CellIndex+1], cells[in.CellIndex:]...)
			cells[in.CellIndex] = newCell
		}

	case "delete":
		if in.CellIndex >= len(cells) {
			return &Result{Content: fmt.Sprintf("Cell index %d out of range (has %d cells)", in.CellIndex, len(cells)), IsError: true}, nil
		}
		cells = append(cells[:in.CellIndex], cells[in.CellIndex+1:]...)

	case "update":
		if in.CellIndex >= len(cells) {
			return &Result{Content: fmt.Sprintf("Cell index %d out of range (has %d cells)", in.CellIndex, len(cells)), IsError: true}, nil
		}
		cell, ok := cells[in.CellIndex].(map[string]interface{})
		if !ok {
			return &Result{Content: "Invalid cell format", IsError: true}, nil
		}
		cell["source"] = []string{in.NewContents}

	default:
		return &Result{Content: fmt.Sprintf("Unknown action: %s", in.Action), IsError: true}, nil
	}

	notebook["cells"] = cells

	// Write back
	output, err := json.MarshalIndent(notebook, "", " ")
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to serialize notebook: %v", err), IsError: true}, nil
	}

	if err := os.WriteFile(in.FilePath, output, 0644); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to write notebook: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Notebook %s: cell %d", in.Action, in.CellIndex)}, nil
}
