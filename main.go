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
	"eve-flipper/internal/db"
	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

//go:embed frontend/dist/*
var frontendFS embed.FS

func main() {
	port := flag.Int("port", 13370, "HTTP server port")
	flag.Parse()

	wd, _ := os.Getwd()
	dataDir := filepath.Join(wd, "data")
	os.MkdirAll(dataDir, 0755)

	// Open SQLite database
	database, err := db.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Database error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Migrate config.json → SQLite (if exists)
	database.MigrateFromJSON()

	// Load config from SQLite
	cfg := database.LoadConfig()

	esiClient := esi.NewClient(database)
	srv := api.NewServer(cfg, esiClient, database)

	// Load SDE in background
	go func() {
		data, err := sde.Load(dataDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "SDE load error: %v\n", err)
			return
		}
		srv.SetSDE(data)
		fmt.Println("SDE loaded, scanner ready")
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
	fmt.Printf("EVE Flipper listening on http://%s\n", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
