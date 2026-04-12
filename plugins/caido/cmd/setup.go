package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	caido "github.com/caido-community/sdk-go"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup for Caido connection",
	Long:  "Configure Caido URL and Personal Access Token (PAT)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create a single scanner for all input
		scanner := bufio.NewScanner(os.Stdin)
		
		// Prompt for URL
		fmt.Print("Caido URL [http://127.0.0.1:8080]: ")
		if !scanner.Scan() {
			return fmt.Errorf("failed to read input")
		}
		url := strings.TrimSpace(scanner.Text())
		if url == "" {
			url = "http://127.0.0.1:8080"
		}

		// Test URL reachability without auth
		fmt.Println("Testing URL reachability...")
		testClient, err := caido.NewClient(caido.Options{
			URL:  url,
			Auth: caido.PATAuth(""),
		})
		if err != nil {
			ErrOut(fmt.Sprintf("failed to create client: %s", err.Error()))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = testClient.Health(ctx)
		if err != nil {
			ErrOut(fmt.Sprintf("failed to reach Caido at %s: %s", url, err.Error()))
		}
		fmt.Println("✓ URL is reachable")

		// Prompt for PAT
		fmt.Println("\nOpen Caido → Settings → Developer → Personal Access Tokens → New")
		fmt.Print("Enter PAT: ")

		// Read PAT without echo (if TTY) or from scanner
		var pat string
		stdinFd := int(os.Stdin.Fd())
		if term.IsTerminal(stdinFd) {
			patBytes, err := term.ReadPassword(stdinFd)
			if err != nil {
				ErrOut(fmt.Sprintf("failed to read PAT: %s", err.Error()))
			}
			fmt.Println() // New line after hidden input
			pat = strings.TrimSpace(string(patBytes))
		} else {
			// Not a TTY (piped input), use scanner
			if !scanner.Scan() {
				ErrOut("failed to read PAT from input")
			}
			pat = strings.TrimSpace(scanner.Text())
		}
		if pat == "" {
			ErrOut("PAT cannot be empty")
		}

		// Test PAT authentication
		fmt.Println("Testing PAT authentication...")
		authClient, err := caido.NewClient(caido.Options{
			URL:  url,
			Auth: caido.PATAuth(pat),
		})
		if err != nil {
			ErrOut(fmt.Sprintf("failed to create authenticated client: %s", err.Error()))
		}

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		health, err := authClient.Health(ctx)
		if err != nil {
			ErrOut(fmt.Sprintf("PAT authentication failed: %s", err.Error()))
		}

		// Write config file
		configDir := filepath.Join(os.ExpandEnv("$HOME"), ".claudio", "plugins")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			ErrOut(fmt.Sprintf("failed to create config directory: %s", err.Error()))
		}

		configPath := filepath.Join(configDir, "caido.json")
		cfg := map[string]interface{}{
			"URL":       url,
			"PAT":       pat,
			"BodyLimit": 2000,
		}

		data, err := json.Marshal(cfg)
		if err != nil {
			ErrOut(fmt.Sprintf("failed to marshal config: %s", err.Error()))
		}

		if err := os.WriteFile(configPath, data, 0600); err != nil {
			ErrOut(fmt.Sprintf("failed to write config file: %s", err.Error()))
		}

		fmt.Printf("Connected to Caido %s. Plugin ready.\n", health.Version)
		return nil
	},
}

func init() {
	Root.AddCommand(setupCmd)
}
