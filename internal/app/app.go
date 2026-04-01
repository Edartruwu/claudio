package app

import (
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/auth"
	authstorage "github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// App holds all shared application dependencies.
type App struct {
	Config   *config.Settings
	Bus      *bus.Bus
	Storage  authstorage.SecureStorage
	Auth     *auth.Resolver
	API      *api.Client
	DB       *storage.DB
	Tools    *tools.Registry
}

// New creates a new App with all dependencies wired up.
func New(settings *config.Settings) (*App, error) {
	if err := config.EnsureDirs(); err != nil {
		return nil, err
	}

	eventBus := bus.New()
	store := authstorage.NewDefaultStorage()
	resolver := auth.NewResolver(store)

	// Open SQLite database
	db, err := storage.Open(config.GetPaths().DB)
	if err != nil {
		return nil, err
	}

	var apiOpts []api.ClientOption
	if settings.APIBaseURL != "" {
		apiOpts = append(apiOpts, api.WithBaseURL(settings.APIBaseURL))
	}
	if settings.Model != "" {
		apiOpts = append(apiOpts, api.WithModel(settings.Model))
	}

	apiClient := api.NewClient(resolver, apiOpts...)

	// Register core tools
	registry := tools.DefaultRegistry()

	return &App{
		Config:  settings,
		Bus:     eventBus,
		Storage: store,
		Auth:    resolver,
		API:     apiClient,
		DB:      db,
		Tools:   registry,
	}, nil
}

// Close cleans up resources.
func (a *App) Close() error {
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}
