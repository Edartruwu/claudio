package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/web"
)

var (
	flagWebPort     int
	flagWebHost     string
	flagWebPassword string
	flagWebAgent    string
	flagWebTeam     string
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start the Claudio web UI",
	Long:  `Starts a browser-based UI for Claudio with project management, chat, and streaming responses.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagWebPassword == "" {
			fmt.Fprintln(os.Stderr, "Error: --password is required")
			os.Exit(1)
		}

		// Load skills registry for web UI skill dispatch
		skillsRegistry := skills.NewRegistry()
		skills.LoadBundled(skillsRegistry)

		srv := web.New(web.Config{
			Port:     flagWebPort,
			Password: flagWebPassword,
			Version:  Version,
			Agent:    flagWebAgent,
			Team:     flagWebTeam,
		}, skillsRegistry)

		return srv.Start()
	},
}

func init() {
	webCmd.Flags().IntVar(&flagWebPort, "port", 0, "Port to listen on (0 = random)")
	webCmd.Flags().StringVar(&flagWebHost, "host", "0.0.0.0", "Host/IP to bind to")
	webCmd.Flags().StringVar(&flagWebPassword, "password", "", "Password for web UI access (required)")
	webCmd.Flags().StringVar(&flagWebAgent, "agent", "", "Agent type to start with (optional)")
	webCmd.Flags().StringVar(&flagWebTeam, "team", "", "Team template to start with (optional)")
	rootCmd.AddCommand(webCmd)
}
