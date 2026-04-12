package cmd

import (
	caido "github.com/caido-community/sdk-go"
	"github.com/spf13/cobra"

	"github.com/Abraxas-365/claudio-plugin-caido/client"
	"github.com/Abraxas-365/claudio-plugin-caido/config"
)

// Package-level vars accessible to subcommands
var (
	Cfg    *config.Config
	Client *caido.Client
)

// Root is the root cobra command
var Root = &cobra.Command{
	Use:   "caido",
	Short: "Caido proxy control",
	Long:  "Query HTTP history, replay requests, manage findings, intercept traffic",
	// No action on root — subcommands only
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load config
		var err error
		Cfg, err = config.Load()
		if err != nil {
			ErrOut("failed to load config: " + err.Error())
		}

		// Create client
		Client, err = client.New(Cfg)
		if err != nil {
			ErrOut("failed to connect to Caido: " + err.Error())
		}

		return nil
	},
}

func init() {
	// Subcommands will be added here in later tasks
}
