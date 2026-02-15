package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"eve-flipper/internal/api"
	"eve-flipper/internal/auth"
	"eve-flipper/internal/db"
	"eve-flipper/internal/esi"
	"eve-flipper/internal/logger"
	"eve-flipper/internal/sde"
)

var version = "dev"

// defaultESIClientID and defaultESIClientSecret are populated for official
// release builds via -ldflags (see .github/workflows/release.yml). For local
// development and source builds they stay empty so SSO is effectively
// disabled unless ESI_CLIENT_ID / ESI_CLIENT_SECRET are provided via env.
var defaultESIClientID = ""
var defaultESIClientSecret = ""
var defaultESICallbackURL = "http://localhost:13370/api/auth/callback"

// loadDotEnv loads environment variables from a local .env file so that
// double-clicked binaries (without a shell) can still use ESI_* settings.
// Order of lookup:
//  1. ./.env (current working directory)
//  2. <binary-dir>/.env
//
// Existing OS env vars are NOT overridden.
func loadDotEnv() {
	paths := []string{".env"}

	if exePath, err := os.Executable(); err == nil {
		if exeDir := filepath.Dir(exePath); exeDir != "" {
			paths = append(paths, filepath.Join(exeDir, ".env"))
		}
	}

	seen := make(map[string]bool)

	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true

		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			l := strings.TrimSpace(line)
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			parts := strings.SplitN(l, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if key == "" {
				continue
			}
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

//go:embed frontend/dist/*
var frontendFS embed.FS

func main() {
	// Load .env for double-clicked binaries / local builds. This is a no-op
	// when the file is absent, and never overrides existing OS env vars.
	loadDotEnv()

	port := flag.Int("port", 13370, "HTTP server port")
	host := flag.String("host", "127.0.0.1", "Host to bind to (use 0.0.0.0 to allow LAN/remote access)")
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

	// Cleanup old market history to prevent unbounded DB growth
	database.CleanupOldHistory()

	// Load config from SQLite
	cfg := database.LoadConfig()

	esiClient := esi.NewClient(database)
	esiClient.LoadEVERefStructures() // background fetch of public structure names

	// ESI SSO config (from env vars or injected defaults for official builds).
	clientID := envOrDefault("ESI_CLIENT_ID", defaultESIClientID)
	clientSecret := envOrDefault("ESI_CLIENT_SECRET", defaultESIClientSecret)
	callbackURL := envOrDefault("ESI_CALLBACK_URL", defaultESICallbackURL)

	var ssoConfig *auth.SSOConfig
	if clientID != "" && clientSecret != "" {
		ssoConfig = &auth.SSOConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			CallbackURL:  callbackURL,
			Scopes: "esi-location.read_location.v1 esi-skills.read_skills.v1 esi-skills.read_skillqueue.v1 esi-wallet.read_character_wallet.v1 esi-assets.read_assets.v1 esi-markets.structure_markets.v1 esi-universe.read_structures.v1 esi-markets.read_character_orders.v1" +
				" esi-characters.read_corporation_roles.v1 esi-wallet.read_corporation_wallets.v1 esi-corporations.read_corporation_membership.v1 esi-industry.read_corporation_jobs.v1 esi-industry.read_corporation_mining.v1 esi-markets.read_corporation_orders.v1 esi-corporations.read_divisions.v1 esi-corporations.track_members.v1" +
				" esi-ui.open_window.v1 esi-ui.write_waypoint.v1",
		}
	} else {
		logger.Info("SSO", "EVE SSO not configured (missing ESI_CLIENT_ID / ESI_CLIENT_SECRET)")
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

	addr := fmt.Sprintf("%s:%d", *host, *port)
	logger.Server(addr)

	httpServer := &http.Server{Addr: addr, Handler: handler}

	// Graceful shutdown on SIGINT / SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		logger.Info("Server", "Shutting down gracefully...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Server", fmt.Sprintf("Shutdown error: %v", err))
		}
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Server", fmt.Sprintf("Failed: %v", err))
		os.Exit(1)
	}
	logger.Info("Server", "Stopped")
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
