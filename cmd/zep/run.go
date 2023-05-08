package cmd

import (
	"fmt"
	"github.com/danielchalef/zep/config"
	"github.com/danielchalef/zep/pkg/extractors"
	"github.com/danielchalef/zep/pkg/llms"
	"github.com/danielchalef/zep/pkg/memorystore"
	"github.com/danielchalef/zep/pkg/models"
	"github.com/danielchalef/zep/pkg/server"
	"os"
	"os/signal"
	"syscall"
)

const (
	ErrMemoryStoreTypeNotSet = "memory_store.type must be set"
	ErrPostgresDSNNotSet     = "memory_store.postgres.dsn must be set"
	MemoryStoreTypePostgres  = "postgres"
)

// run is the entrypoint for the zep server
func run() {
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		log.Fatalf("Error loading config: %s", err)
	}

	config.SetLogLevel(cfg)
	appState := NewAppState(cfg)

	// Init the extractors, which will register themselves with the MemoryStore
	extractors.Initialize(appState)

	srv := server.Create(appState)

	log.Info("Listening on: ", srv.Addr)
	err = srv.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

// NewAppState creates an AppState struct from the config file / ENV, initializes the memory store,
// and creates the OpenAI client
func NewAppState(cfg *config.Config) *models.AppState {
	appState := &models.AppState{
		OpenAIClient: llms.CreateOpenAIClient(cfg),
		Config:       cfg,
	}

	initializeMemoryStore(appState)
	setupSignalHandler(appState)

	return appState
}

// initializeMemoryStore initializes the memory store based on the config file / ENV
func initializeMemoryStore(appState *models.AppState) {
	if appState.Config.MemoryStore.Type == "" {
		log.Fatal(ErrMemoryStoreTypeNotSet)
	}

	switch appState.Config.MemoryStore.Type {
	case MemoryStoreTypePostgres:
		if appState.Config.MemoryStore.Postgres.DSN == "" {
			log.Fatal(ErrPostgresDSNNotSet)
		}
		db := memorystore.NewPostgresConn(appState.Config.MemoryStore.Postgres.DSN)
		memoryStore, err := memorystore.NewPostgresMemoryStore(appState, db)
		if err != nil {
			log.Fatal(err)
		}
		appState.MemoryStore = memoryStore
	default:
		log.Fatal(
			fmt.Sprintf(
				"memory_store.type (%s) is not supported",
				appState.Config.MemoryStore.Type,
			),
		)
	}

	log.Info("Using memory store: ", appState.Config.MemoryStore.Type)
}

// setupSignalHandler sets up a signal handler to close the MemoryStore connection on termination
func setupSignalHandler(appState *models.AppState) {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalCh
		if err := appState.MemoryStore.Close(); err != nil {
			log.Errorf("Error closing MemoryStore connection: %v", err)
		}
		os.Exit(0)
	}()
}