package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var envsCmd = &cobra.Command{
	Use:   "envs",
	Short: "List environments and variables",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// List all environments
		resp, err := Client.Environments.List(ctx)
		if err != nil {
			ErrOut("failed to list environments: " + err.Error())
		}

		// Extract environments with variables
		var environments []map[string]interface{}
		for _, env := range resp.Environments {
			var variables []map[string]interface{}
			for _, variable := range env.Variables {
				v := map[string]interface{}{
					"name":  variable.Name,
					"value": variable.Value,
					"kind":  variable.Kind,
				}
				variables = append(variables, v)
			}
			e := map[string]interface{}{
				"id":        env.Id,
				"name":      env.Name,
				"version":   env.Version,
				"variables": variables,
			}
			environments = append(environments, e)
		}

		JSONOut(environments)
		return nil
	},
}

var envSelectCmd = &cobra.Command{
	Use:   "env-select <id>",
	Short: "Switch environment (empty string to deselect)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		ctx := context.Background()

		// Select environment (nil pointer to deselect)
		var idPtr *string
		if id != "" {
			idPtr = &id
		}

		resp, err := Client.Environments.Select(ctx, idPtr)
		if err != nil {
			ErrOut("failed to select environment: " + err.Error())
		}

		var out map[string]interface{}
		if id == "" {
			out = map[string]interface{}{
				"success": true,
				"message": "deselected environment",
			}
		} else {
			if resp.SelectEnvironment.Environment == nil {
				ErrOut("failed to select environment: no environment in response")
			}
			out = map[string]interface{}{
				"id":   resp.SelectEnvironment.Environment.Id,
				"name": resp.SelectEnvironment.Environment.Name,
			}
		}

		JSONOut(out)
		return nil
	},
}

func init() {
	// envs command
	Root.AddCommand(envsCmd)

	// env-select command
	Root.AddCommand(envSelectCmd)
}
