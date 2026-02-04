package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"eve-flipper/internal/api"
	"eve-flipper/internal/auth"
	"eve-flipper/internal/db"
	"eve-flipper/internal/esi"
	"eve-flipper/internal/logger"
	"eve-flipper/internal/sde"
)

var version = "dev"

//go:embed frontend/dist/*
var frontendFS embed.FS

func main() {
	port := flag.Int("port", 13370, "HTTP server port")
	flag.Parse()

	logger.Banner(version)

	wd, _ := os.Getwd()
	dataDir := filepath.Join(wd, "data")
	os.MkdirAll(dataDir, 0755)

	// Open SQLite database
	database, err := db.Open()
	if err != nil {
		logger.Error("DB", fmt.Sprintf("Failed to open database: %v", err))
		os.Exit(1)
	}
	defer database.Close()

	// Migrate config.json → SQLite (if exists)
	database.MigrateFromJSON()

	// Load config from SQLite
	cfg := database.LoadConfig()

	esiClient := esi.NewClient(database)

	// ESI SSO config (from env vars or defaults)
	ssoConfig := &auth.SSOConfig{
		ClientID:     envOrDefault("ESI_CLIENT_ID", "4aa8473d2a53457b92e368e823edcb1b"),
		ClientSecret: envOrDefault("ESI_CLIENT_SECRET", "eat_qHE5tc6su6yTdKMIvsmMDDEFruV55GOo_3xtaoY"),
		CallbackURL:  envOrDefault("ESI_CALLBACK_URL", "http://localhost:13370/api/auth/callback"),
		Scopes:       "esi-location.read_location.v1 esi-skills.read_skills.v1 esi-skills.read_skillqueue.v1 esi-wallet.read_character_wallet.v1 esi-assets.read_assets.v1 esi-markets.structure_markets.v1 esi-markets.read_character_orders.v1",
	}
	sessions := auth.NewSessionStore(database.SqlDB())

	srv := api.NewServer(cfg, esiClient, database, ssoConfig, sessions)

	// Load SDE in background
	go func() {
		data, err := sde.Load(dataDir)
		if err != nil {
			logger.Error("SDE", fmt.Sprintf("Load failed: %v", err))
			return
		}
		srv.SetSDE(data)
		logger.Success("SDE", "Scanner ready")
	}()

	// Combine API + embedded frontend into a single handler
	apiHandler := srv.Handler()
	frontendContent, _ := fs.Sub(frontendFS, "frontend/dist")
	fileServer := http.FileServer(http.FS(frontendContent))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		// Try static file, fall back to index.html (SPA)
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(frontendContent, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	logger.Server(addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Error("Server", fmt.Sprintf("Failed: %v", err))
		os.Exit(1)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
