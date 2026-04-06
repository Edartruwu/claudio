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
			Host:     flagWebHost,
			Password: flagWebPassword,
			Version:  Version,
		}, skillsRegistry)

		fmt.Printf("Starting Claudio Web UI on http://%s:%d\n", flagWebHost, flagWebPort)
		fmt.Println("Use --password flag value to log in.")
		return srv.Start()
	},
}

func init() {
	webCmd.Flags().IntVar(&flagWebPort, "port", 8080, "Port to listen on")
	webCmd.Flags().StringVar(&flagWebHost, "host", "0.0.0.0", "Host/IP to bind to")
	webCmd.Flags().StringVar(&flagWebPassword, "password", "", "Password for web UI access (required)")
	rootCmd.AddCommand(webCmd)
}
