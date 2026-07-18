//go:build wails
// +build wails

package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"eve-flipper/internal/api"
	"eve-flipper/internal/auth"
	"eve-flipper/internal/db"
	"eve-flipper/internal/esi"
	"eve-flipper/internal/logger"
	"eve-flipper/internal/sde"
	"eve-flipper/internal/telemetry"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsopts "github.com/wailsapp/wails/v2/pkg/options/windows"
)

var version = "dev"

// defaultESIClientID and defaultESIClientSecret are populated for official
// release builds via -ldflags (see .github/workflows/release.yml). For local
// development and source builds they stay empty so SSO is effectively
// disabled unless ESI_CLIENT_ID / ESI_CLIENT_SECRET are provided via env.
var defaultESIClientID = ""
var defaultESIClientSecret = ""
var defaultESICallbackURL = "http://localhost:13370/api/auth/callback"

//go:embed frontend/dist/*
var wailsFrontendFS embed.FS

type backendRuntime struct {
	httpServer *http.Server
	database   *db.DB
	closeLogs  func()
	baseURL    string
	stopOnce   sync.Once
}

func (r *backendRuntime) Stop(ctx context.Context) {
	r.stopOnce.Do(func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if r.httpServer != nil {
			_ = r.httpServer.Shutdown(shutdownCtx)
		}
		if r.database != nil {
			r.database.Close()
		}
		if r.closeLogs != nil {
			r.closeLogs()
		}
	})
}

func main() {
	loadDotEnv()

	backend, err := startBackend("127.0.0.1", 13370)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start backend for Wails: %v\n", err)
		os.Exit(1)
	}
	defer backend.Stop(context.Background())

	err = wails.Run(&options.App{
		Title:            "EVE Flipper",
		Width:            1100,
		Height:           700,
		MinWidth:         900,
		MinHeight:        600,
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets:  wailsFrontendFS,
			Handler: newBackendProxy(backend.baseURL),
		},
		BackgroundColour: &options.RGBA{R: 13, G: 13, B: 13, A: 255},
		DisableResize:    false,
		Windows: &windowsopts.Options{
			DisableWindowIcon: false,
		},
		OnShutdown: func(ctx context.Context) {
			backend.Stop(ctx)
		},
	})
	if err != nil {
		backend.Stop(context.Background())
		fmt.Fprintf(os.Stderr, "failed to run Wails app: %v\n", err)
		os.Exit(1)
	}
}

func startBackend(host string, preferredPort int) (*backendRuntime, error) {
	// File logs live next to the running binary (release/build folder).
	logDir := "."
	if exePath, err := os.Executable(); err == nil {
		if exeDir := filepath.Dir(exePath); exeDir != "" {
			logDir = exeDir
		}
	}

	closeLogs := func() {}
	if err := logger.InitFileLogging(logDir); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] file logging init failed: %v\n", err)
	} else {
		closeLogs = logger.CloseFileLogging
	}

	logger.Banner(version)
	if prodLog, debugLog := logger.LogFiles(); prodLog != "" || debugLog != "" {
		if prodLog != "" {
			logger.Info("LOG", "Prod log file: "+prodLog)
		}
		if debugLog != "" {
			logger.Info("LOG", "Debug log file: "+debugLog)
		}
	}

	wd, _ := os.Getwd()
	dataDir := filepath.Join(wd, "data")
	_ = os.MkdirAll(dataDir, 0755)

	database, err := db.Open()
	if err != nil {
		closeLogs()
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Migrate config.json -> SQLite (if exists). Cache cleanup runs after
	// startup so large existing databases do not block the desktop UI.
	database.MigrateFromJSON()
	database.CleanupStartupCachesAsync(30 * time.Second)
	cfg := database.LoadConfig()

	listener, port, err := listenOnPreferredOrFreePort(host, preferredPort)
	if err != nil {
		database.Close()
		closeLogs()
		return nil, err
	}
	baseURL := fmt.Sprintf("http://%s:%d", host, port)

	esiClient := esi.NewClient(database)
	esiClient.LoadEVERefStructures()

	clientID := envOrDefault("ESI_CLIENT_ID", defaultESIClientID)
	clientSecret := envOrDefault("ESI_CLIENT_SECRET", defaultESIClientSecret)
	callbackURL := strings.TrimSpace(os.Getenv("ESI_CALLBACK_URL"))
	if callbackURL == "" {
		callbackURL = defaultESICallbackURL
		if preferredPort != 0 && port != preferredPort {
			callbackURL = fmt.Sprintf("http://localhost:%d/api/auth/callback", port)
			logger.Warn("SSO", fmt.Sprintf(
				"Using fallback callback URL %s because preferred desktop backend port %d is unavailable. EVE SSO may require closing the other Eve Flipper process.",
				callbackURL,
				preferredPort,
			))
		}
	}

	var ssoConfig *auth.SSOConfig
	if clientID != "" && clientSecret != "" {
		ssoConfig = &auth.SSOConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			CallbackURL:  callbackURL,
			Scopes: "esi-location.read_location.v1 esi-skills.read_skills.v1 esi-skills.read_skillqueue.v1 esi-wallet.read_character_wallet.v1 esi-assets.read_assets.v1 esi-characters.read_blueprints.v1 esi-industry.read_character_jobs.v1 esi-planets.manage_planets.v1 esi-markets.structure_markets.v1 esi-universe.read_structures.v1 esi-markets.read_character_orders.v1" +
				" esi-characters.read_corporation_roles.v1 esi-wallet.read_corporation_wallets.v1 esi-corporations.read_corporation_membership.v1 esi-industry.read_corporation_jobs.v1 esi-industry.read_corporation_mining.v1 esi-markets.read_corporation_orders.v1 esi-corporations.read_divisions.v1 esi-corporations.track_members.v1" +
				" esi-ui.open_window.v1 esi-ui.write_waypoint.v1",
		}
	} else {
		logger.Info("SSO", "EVE SSO not configured (missing ESI_CLIENT_ID / ESI_CLIENT_SECRET)")
	}
	sessions := auth.NewSessionStore(database.SqlDB())
	database.SetPrivacyCodec(sessions.Vault())

	srv := api.NewServer(cfg, esiClient, database, ssoConfig, sessions)
	srv.SetAppVersion(version)
	srv.SetAppFlavor("desktop")
	srv.SetTelemetry(telemetry.NewFromEnv())

	// Load SDE in background.
	go func() {
		data, err := sde.Load(dataDir)
		if err != nil {
			logger.Error("SDE", fmt.Sprintf("Load failed: %v", err))
			return
		}
		missingShipVolumes := prepareShipPackagedVolumes(dataDir, data)
		srv.SetSDE(data)
		logger.Success("SDE", "Scanner ready")
		refreshShipPackagedVolumesInBackground(dataDir, missingShipVolumes, esiClient)
	}()

	addr := fmt.Sprintf("%s:%d", host, port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if err := waitForBackendReady(baseURL, 15*time.Second, errCh); err != nil {
		_ = httpServer.Close()
		database.Close()
		closeLogs()
		return nil, err
	}

	logger.Server(addr)

	return &backendRuntime{
		httpServer: httpServer,
		database:   database,
		closeLogs:  closeLogs,
		baseURL:    baseURL,
	}, nil
}

func listenOnPreferredOrFreePort(host string, preferredPort int) (net.Listener, int, error) {
	listen := func(port int) (net.Listener, int, error) {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return nil, 0, err
		}
		tcpAddr, ok := ln.Addr().(*net.TCPAddr)
		if !ok {
			_ = ln.Close()
			return nil, 0, fmt.Errorf("unexpected listener address: %s", ln.Addr())
		}
		return ln, tcpAddr.Port, nil
	}

	ln, port, err := listen(preferredPort)
	if err == nil {
		return ln, port, nil
	}
	if preferredPort == 0 {
		return nil, 0, fmt.Errorf("listen on %s: %w", net.JoinHostPort(host, strconv.Itoa(preferredPort)), err)
	}

	fallback, fallbackPort, fallbackErr := listen(0)
	if fallbackErr != nil {
		return nil, 0, fmt.Errorf(
			"listen on preferred %s failed: %w; fallback free port failed: %v",
			net.JoinHostPort(host, strconv.Itoa(preferredPort)),
			err,
			fallbackErr,
		)
	}
	logger.Warn("SERVER", fmt.Sprintf(
		"Preferred desktop backend port %d is unavailable; using %s. Stop the other Eve Flipper process if EVE SSO callback on %d is needed.",
		preferredPort,
		net.JoinHostPort(host, strconv.Itoa(fallbackPort)),
		preferredPort,
	))
	return fallback, fallbackPort, nil
}

func newBackendProxy(baseURL string) http.Handler {
	target, err := url.Parse(baseURL)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "backend proxy is not configured", http.StatusBadGateway)
		})
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "backend unavailable: "+err.Error(), http.StatusBadGateway)
	}
	return proxy
}

func waitForBackendReady(baseURL string, timeout time.Duration, errCh <-chan error) error {
	client := &http.Client{Timeout: 700 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	statusURL := baseURL + "/api/status"

	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			return fmt.Errorf("backend failed during startup: %w", err)
		default:
		}

		resp, err := client.Get(statusURL)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}

	select {
	case err := <-errCh:
		return fmt.Errorf("backend failed during startup: %w", err)
	default:
	}
	return fmt.Errorf("backend startup timeout after %s", timeout)
}

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
				_ = os.Setenv(key, val)
			}
		}
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
