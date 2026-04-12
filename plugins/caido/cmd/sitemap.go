package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var sitemapCmd = &cobra.Command{
	Use:   "sitemap",
	Short: "Browse sitemap endpoint hierarchy",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		resp, err := Client.Sitemap.ListRootEntries(ctx, nil)
		if err != nil {
			ErrOut("failed to list sitemap entries: " + err.Error())
		}

		type SitemapEntry struct {
			ID             string `json:"id"`
			Label          string `json:"label"`
			Kind           string `json:"kind"`
			HasDescendants bool   `json:"has_descendants"`
		}

		var entries []SitemapEntry
		for _, edge := range resp.SitemapRootEntries.Edges {
			entries = append(entries, SitemapEntry{
				ID:             edge.Node.Id,
				Label:          edge.Node.Label,
				Kind:           string(edge.Node.Kind),
				HasDescendants: edge.Node.HasDescendants,
			})
		}

		JSONOut(entries)
		return nil
	},
}

func init() {
	Root.AddCommand(sitemapCmd)
}
