package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
)

// maxImageDimension is Claude's vision API limit per dimension.
const maxImageDimension = 7000

// cropImageIfNeeded decodes a PNG, crops it to maxImageDimension height if
// taller, and re-encodes to PNG bytes. Returns original bytes unchanged if
// within limits or decode fails (non-fatal — let the API return the error).
func cropImageIfNeeded(data []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	b := img.Bounds()
	if b.Dy() <= maxImageDimension && b.Dx() <= maxImageDimension {
		return data
	}
	// Crop to maxImageDimension in each axis.
	maxW := b.Dx()
	if maxW > maxImageDimension {
		maxW = maxImageDimension
	}
	maxH := b.Dy()
	if maxH > maxImageDimension {
		maxH = maxImageDimension
	}
	cropped := image.NewRGBA(image.Rect(0, 0, maxW, maxH))
	draw.Draw(cropped, cropped.Bounds(), img, b.Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, cropped); err != nil {
		return data
	}
	return buf.Bytes()
}

// maxHTMLContext is the max chars of HTML source included in the critic prompt.
const maxHTMLContext = 8000

// VerifyMockupTool sends a rendered screenshot to a vision-capable LLM
// that scores it against 7 quality dimensions. Acts as an adversarial
// critic — uses a different model than the one generating the mockup.
type VerifyMockupTool struct {
	designsDir string
	client     *api.Client
	model      string // vision-capable model for critique
}

// NewVerifyMockupTool creates a VerifyMockupTool.
// client is used for LLM vision calls, model specifies the critic model.
func NewVerifyMockupTool(designsDir string, client *api.Client, model string) *VerifyMockupTool {
	return &VerifyMockupTool{designsDir: designsDir, client: client, model: model}
}

// VerifyMockupInput is the JSON input schema.
type VerifyMockupInput struct {
	ScreenshotPath string `json:"screenshot_path"` // path to PNG from RenderMockup
	DesignBrief    string `json:"design_brief"`     // original user requirement
	HTMLPath       string `json:"html_path"`         // source HTML for context
}

// VerifyMockupOutput is the structured result from the vision critic.
type VerifyMockupOutput struct {
	OverallScore    int              `json:"overall_score"`    // 0-100
	Pass            bool             `json:"pass"`             // score >= 75 AND no blocking issues
	Dimensions      []DimensionScore `json:"dimensions"`
	BlockingIssues  []string         `json:"blocking_issues"`
	Suggestions     []string         `json:"suggestions"`
	RawCritiqueText string           `json:"raw_critique"`
}

// DimensionScore holds one dimension's evaluation.
type DimensionScore struct {
	Name        string `json:"name"`
	Score       int    `json:"score"`       // 0-100
	Observation string `json:"observation"`
}

func (t *VerifyMockupTool) Name() string { return "VerifyMockup" }

func (t *VerifyMockupTool) Description() string {
	return `Evaluate a rendered mockup screenshot against design quality standards.

Sends the screenshot to a vision-capable LLM acting as an adversarial critic.
Scores 7 dimensions (0-100): Visual Hierarchy, Spacing & Rhythm, Typography,
Color & Contrast, Component Completeness, Interaction Affordance, Responsive
Readiness. Returns structured JSON with per-dimension scores, blocking issues,
and actionable suggestions.

Pass threshold: overall_score >= 75 AND no blocking issues.`
}

func (t *VerifyMockupTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
	"type": "object",
	"properties": {
		"screenshot_path": {
			"type": "string",
			"description": "Absolute path to the PNG screenshot from RenderMockup."
		},
		"design_brief": {
			"type": "string",
			"description": "The original design requirement / user prompt."
		},
		"html_path": {
			"type": "string",
			"description": "Path to the source HTML file (included as context for the critic)."
		}
	},
	"required": ["screenshot_path", "design_brief"]
}`)
}

func (t *VerifyMockupTool) IsReadOnly() bool { return true }

func (t *VerifyMockupTool) RequiresApproval(_ json.RawMessage) bool { return false }

// criticModel returns the model to use for critique.
// Priority: CLAUDIO_DESIGN_CRITIC_MODEL env > constructor model param.
func (t *VerifyMockupTool) criticModel() string {
	if m := os.Getenv("CLAUDIO_DESIGN_CRITIC_MODEL"); m != "" {
		return m
	}
	if t.model != "" {
		return t.model
	}
	return "claude-haiku-4-5-20251001"
}

func (t *VerifyMockupTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in VerifyMockupInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.ScreenshotPath == "" {
		return &Result{Content: "screenshot_path is required", IsError: true}, nil
	}
	if in.DesignBrief == "" {
		return &Result{Content: "design_brief is required", IsError: true}, nil
	}

	// Remap paths for worktree support.
	screenshotPath := RemapPathForWorktree(ctx, in.ScreenshotPath)
	htmlPath := ""
	if in.HTMLPath != "" {
		htmlPath = RemapPathForWorktree(ctx, in.HTMLPath)
	}

	// 1. Read screenshot → base64.
	imgBytes, err := os.ReadFile(screenshotPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to read screenshot %q: %v", screenshotPath, err), IsError: true}, nil
	}
	imgBytes = cropImageIfNeeded(imgBytes)
	imgBase64 := base64.StdEncoding.EncodeToString(imgBytes)

	// 2. Read HTML source for context (optional, truncated).
	htmlContext := "(no HTML source provided)"
	if htmlPath != "" {
		if htmlBytes, err := os.ReadFile(htmlPath); err == nil {
			htmlContext = string(htmlBytes)
			if len(htmlContext) > maxHTMLContext {
				htmlContext = htmlContext[:maxHTMLContext] + "\n... (truncated)"
			}
		}
	}

	// 3. Build prompt from template.
	prompt := verifyCriticPrompt
	prompt = strings.ReplaceAll(prompt, "{{DESIGN_BRIEF}}", in.DesignBrief)
	prompt = strings.ReplaceAll(prompt, "{{HTML_CONTEXT}}", htmlContext)

	// 4. Build user message with image + text.
	contentBlocks := []api.UserContentBlock{
		api.NewImageBlock("image/png", imgBase64),
		api.NewTextBlock("Evaluate this mockup screenshot. Respond with JSON only."),
	}
	contentJSON, err := json.Marshal(contentBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content blocks: %w", err)
	}

	messages := []api.Message{
		{Role: "user", Content: json.RawMessage(contentJSON)},
	}

	// 5. Call LLM.
	model := t.criticModel()
	resp, err := t.client.SendMessage(ctx, &api.MessagesRequest{
		Model:     model,
		System:    prompt,
		Messages:  messages,
		MaxTokens: 4096,
	})
	if err != nil {
		return &Result{
			Content: fmt.Sprintf("Vision critic LLM call failed: %v", err),
			IsError: true,
		}, nil
	}

	// 6. Extract text from response.
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			responseText += block.Text
		}
	}

	if responseText == "" {
		return &Result{Content: "Vision critic returned empty response", IsError: true}, nil
	}

	// 7. Parse JSON response.
	// Strip markdown code fences if LLM wraps response.
	cleaned := strings.TrimSpace(responseText)
	if strings.HasPrefix(cleaned, "```") {
		// Remove opening fence (```json or ```)
		if idx := strings.Index(cleaned, "\n"); idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	var output VerifyMockupOutput
	if err := json.Unmarshal([]byte(cleaned), &output); err != nil {
		// Return raw text so caller can still see critique.
		return &Result{
			Content: fmt.Sprintf("Failed to parse critic JSON (raw response below):\n%s\n\nParse error: %v", responseText, err),
			IsError: true,
		}, nil
	}

	// Ensure slices are non-nil for clean JSON.
	if output.Dimensions == nil {
		output.Dimensions = []DimensionScore{}
	}
	if output.BlockingIssues == nil {
		output.BlockingIssues = []string{}
	}
	if output.Suggestions == nil {
		output.Suggestions = []string{}
	}

	outJSON, _ := json.MarshalIndent(output, "", "  ")
	return &Result{Content: string(outJSON)}, nil
}
