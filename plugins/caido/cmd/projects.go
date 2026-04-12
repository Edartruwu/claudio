package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List all projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		listResp, err := Client.Projects.List(ctx)
		if err != nil {
			ErrOut("failed to list projects: " + err.Error())
		}

		currentResp, err := Client.Projects.GetCurrent(ctx)
		if err != nil {
			ErrOut("failed to get current project: " + err.Error())
		}

		var currentID string
		if currentResp.CurrentProject != nil {
			currentID = currentResp.CurrentProject.Project.Id
		}

		type ProjectItem struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Active   bool   `json:"active"`
			Status   string `json:"status"`
			Temporary bool   `json:"temporary"`
		}

		var projects []ProjectItem
		for _, proj := range listResp.Projects {
			projects = append(projects, ProjectItem{
				ID:       proj.Id,
				Name:     proj.Name,
				Active:   proj.Id == currentID,
				Status:   string(proj.Status),
				Temporary: proj.Temporary,
			})
		}

		JSONOut(projects)
		return nil
	},
}

var projectSelectCmd = &cobra.Command{
	Use:   "project-select <id>",
	Short: "Switch to a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		projectID := args[0]

		resp, err := Client.Projects.Select(ctx, projectID)
		if err != nil {
			ErrOut("failed to select project: " + err.Error())
		}

		type Selected struct {
			Selected string `json:"selected"`
		}

		out := Selected{
			Selected: resp.SelectProject.CurrentProject.Project.Id,
		}

		JSONOut(out)
		return nil
	},
}

func init() {
	Root.AddCommand(projectsCmd)
	Root.AddCommand(projectSelectCmd)
}
