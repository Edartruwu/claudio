package cmd

import (
	"context"

	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/spf13/cobra"
)

var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List workflows",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// List workflows
		resp, err := Client.Workflows.List(ctx)
		if err != nil {
			ErrOut("failed to list workflows: " + err.Error())
		}

		// Extract workflows from response
		var workflows []map[string]interface{}
		for _, workflow := range resp.Workflows {
			w := map[string]interface{}{
				"id":      workflow.Id,
				"name":    workflow.Name,
				"enabled": workflow.Enabled,
			}
			workflows = append(workflows, w)
		}

		JSONOut(workflows)
		return nil
	},
}

var workflowRunCmd = &cobra.Command{
	Use:   "workflow-run",
	Short: "Run a workflow",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("id")
		workflowType, _ := cmd.Flags().GetString("type")
		requestID, _ := cmd.Flags().GetString("request-id")
		input, _ := cmd.Flags().GetString("input")

		// Validate required flags
		if id == "" {
			ErrOut("--id is required")
		}
		if workflowType == "" {
			ErrOut("--type is required")
		}

		// Cross-validate type vs required flags
		if workflowType == "active" {
			if requestID == "" {
				ErrOut("--request-id is required when --type=active")
			}
		} else if workflowType == "convert" {
			if input == "" {
				ErrOut("--input is required when --type=convert")
			}
		} else {
			ErrOut("--type must be 'active' or 'convert'")
		}

		ctx := context.Background()

		// Run the appropriate workflow type
		var result interface{}

		if workflowType == "active" {
			runInput := &gen.RunActiveWorkflowInput{
				RequestId: requestID,
			}
			resp, err := Client.Workflows.RunActive(ctx, id, runInput)
			if err != nil {
				ErrOut("failed to run active workflow: " + err.Error())
			}
			if resp.RunActiveWorkflow.Error != nil {
				ErrOut("workflow execution failed")
			}
			result = map[string]interface{}{
				"success": true,
			}
		} else {
			resp, err := Client.Workflows.RunConvert(ctx, id, input)
			if err != nil {
				ErrOut("failed to run convert workflow: " + err.Error())
			}
			if resp.RunConvertWorkflow.Error != nil {
				ErrOut("workflow execution failed")
			}
			output := ""
			if resp.RunConvertWorkflow.Output != nil {
				output = *resp.RunConvertWorkflow.Output
			}
			result = map[string]interface{}{
				"output": output,
			}
		}

		JSONOut(result)
		return nil
	},
}

var workflowToggleCmd = &cobra.Command{
	Use:   "workflow-toggle",
	Short: "Enable or disable a workflow",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("id")
		enabled, _ := cmd.Flags().GetBool("enabled")

		// Validate required flags
		if id == "" {
			ErrOut("--id is required")
		}

		ctx := context.Background()

		// Toggle workflow
		resp, err := Client.Workflows.Toggle(ctx, id, enabled)
		if err != nil {
			ErrOut("failed to toggle workflow: " + err.Error())
		}

		if resp.ToggleWorkflow.Workflow == nil {
			ErrOut("failed to toggle workflow: no workflow in response")
		}

		workflow := map[string]interface{}{
			"id":      resp.ToggleWorkflow.Workflow.Id,
			"name":    resp.ToggleWorkflow.Workflow.Name,
			"enabled": resp.ToggleWorkflow.Workflow.Enabled,
		}

		JSONOut(workflow)
		return nil
	},
}

func init() {
	// workflows command
	Root.AddCommand(workflowsCmd)

	// workflow-run command
	workflowRunCmd.Flags().StringP("id", "", "", "Workflow ID (required)")
	workflowRunCmd.Flags().StringP("type", "", "", "Workflow type: active or convert (required)")
	workflowRunCmd.Flags().StringP("request-id", "", "", "Request ID for active workflows")
	workflowRunCmd.Flags().StringP("input", "", "", "Input data for convert workflows")
	Root.AddCommand(workflowRunCmd)

	// workflow-toggle command
	workflowToggleCmd.Flags().StringP("id", "", "", "Workflow ID (required)")
	workflowToggleCmd.Flags().BoolP("enabled", "", false, "Enable or disable the workflow")
	Root.AddCommand(workflowToggleCmd)
}
