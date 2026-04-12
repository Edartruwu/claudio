package client

import (
	"context"
	"time"

	caido "github.com/caido-community/sdk-go"

	"github.com/Abraxas-365/claudio-plugin-caido/config"
)

// New creates and connects a Caido client.
// Returns error if connection fails — never panics.
func New(cfg *config.Config) (*caido.Client, error) {
	client, err := caido.NewClient(caido.Options{
		URL:  cfg.URL,
		Auth: caido.PATAuth(cfg.PAT),
	})
	if err != nil {
		return nil, err
	}

	// Connect with 5s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return nil, err
	}

	return client, nil
}
