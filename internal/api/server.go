package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"eve-flipper/internal/auth"
	"eve-flipper/internal/config"
	"eve-flipper/internal/corp"
	"eve-flipper/internal/db"
	"eve-flipper/internal/engine"
	"eve-flipper/internal/esi"
	"eve-flipper/internal/gankcheck"
	"eve-flipper/internal/sde"
	"eve-flipper/internal/zkillboard"
	"golang.org/x/sync/singleflight"
)

// Server is the HTTP API server that connects the ESI client, scanner engine, and database.
type Server struct {
	cfg              *config.Config
	sdeData          *sde.Data
	scanner          *engine.Scanner
	industryAnalyzer *engine.IndustryAnalyzer
	demandAnalyzer   *zkillboard.DemandAnalyzer
	esi              *esi.Client
	db               *db.DB
	sso              *auth.SSOConfig
	sessions         *auth.SessionStore
	mu               sync.RWMutex
	ready            bool
	wikiRAG          *stationAIWikiRAG

	// SSO state: map of CSRF state tokens → (expiry, desktop flag).
	// Supports concurrent login flows from multiple tabs.
	ssoStatesMu sync.Mutex
	ssoStates   map[string]ssoStateEntry

	// Wallet transaction cache for P&L tab (TTL 2 min).
	txnCacheMu          sync.RWMutex
	txnCache            []esi.WalletTransaction
	txnCacheTime        time.Time
	txnCacheCharacterID int64

	// PLEX dashboard cache (TTL 5 min) to avoid hammering ESI with 5 concurrent requests per click.
	plexCacheMu    sync.RWMutex
	plexCache      *engine.PLEXDashboard
	plexCacheTime  time.Time
	plexCacheKey   string // "salesTax_brokerFee_nesE_nesO_omegaUSD"
	plexBuildGroup singleflight.Group
	plexBuildSem   chan struct{} // global limiter for heavy PLEX refreshes

	// Corporation demo provider (initialized on SDE load).
	demoCorpProvider *corp.DemoCorpProvider

	// Gank check route danger analyzer (initialized on SDE load).
	ganker *gankcheck.Checker

	userIDCookieSecret []byte

	authRevisionMu sync.Mutex
	authRevision   map[string]int64

	appVersion string
	appFlavor  string
	updateHTTP *http.Client

	updateSkipMu     sync.RWMutex
	updateSkipByUser map[string]string
}

// ssoStateEntry holds metadata for a pending SSO login flow.
type ssoStateEntry struct {
	ExpiresAt time.Time
	Desktop   bool
	UserID    string
}

const walletTxnCacheTTL = 2 * time.Minute
const plexCacheTTL = 5 * time.Minute
const plexStaleCacheTTL = 30 * time.Minute
const userIDCookieName = "eveflipper_uid"
const userIDHeaderName = "X-EveFlipper-UID"
const userIDCookieMaxAge = 365 * 24 * 60 * 60
const userIDCookieSignatureBytes = 16
const userIDCookieSecretMetaKey = "user_cookie_secret_v1"
const defaultStationAIWikiRepo = "ilyaux/Eve-flipper"
const defaultStationAIPlannerModel = ""
const aiWikiCacheTTL = 30 * time.Minute
const aiWikiErrorCacheTTL = 90 * time.Second
const stationAIWebMaxQueries = 3
const stationAIWebMaxSnippets = 4
const stationAIRuntimeTopItems = 5
const stationAIRuntimeTxnWindowDays = 30
const stationAIHistoryMaxMessages = 16
const stationAIStreamHTTPTimeout = 12 * time.Minute
const stationAIProviderResponseMaxBodyBytes int64 = 4 * 1024 * 1024
const stationAIProviderErrorMaxBodyBytes int64 = 64 * 1024
const stationAIWikiWebhookSecretEnv = "STATION_AI_WIKI_WEBHOOK_SECRET"
const stationAIWikiWebhookRefreshTimeout = 2 * time.Minute
const stationAIMaxTokensLimit = 1_000_000
const industryAnalyzeMaxBodyBytes = 64 * 1024
const industryAnalyzeMaxRuns int32 = 10000
const industryAnalyzeMaxDepth = 20
const industrySearchMaxLimit = 100

type contextKey string

const userIDContextKey contextKey = "user_id"

var aiRepoPartRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type aiWikiCacheEntry struct {
	Body      string
	URL       string
	FetchedAt time.Time
	Err       string
}

type aiKnowledgeSnippet struct {
	SourceLabel  string
	Title        string
	Page         string
	Section      string
	Locale       string
	URL          string
	Content      string
	Score        int
	VectorScore  float64
	KeywordScore float64
	HybridScore  float64
}

type stationAIContextSummary struct {
	TotalRows      int     `json:"total_rows"`
	VisibleRows    int     `json:"visible_rows"`
	HighRiskRows   int     `json:"high_risk_rows"`
	ExtremeRows    int     `json:"extreme_rows"`
	AverageCTS     float64 `json:"avg_cts"`
	AverageMargin  float64 `json:"avg_margin"`
	AverageProfit  float64 `json:"avg_daily_profit"`
	AverageVolume  float64 `json:"avg_daily_volume"`
	ActionableRows int     `json:"actionable_rows"`
}

type stationAIRuntimeItemFlow struct {
	TypeID      int32   `json:"type_id"`
	TypeName    string  `json:"type_name"`
	Trades      int     `json:"trades"`
	Volume      int64   `json:"volume"`
	TurnoverISK float64 `json:"turnover_isk"`
}

type stationAIRuntimeContext struct {
	Available             bool                         `json:"available"`
	CharacterID           int64                        `json:"character_id"`
	CharacterName         string                       `json:"character_name"`
	WalletAvailable       bool                         `json:"wallet_available"`
	WalletISK             float64                      `json:"wallet_isk"`
	OrdersAvailable       bool                         `json:"orders_available"`
	ActiveOrders          int                          `json:"active_orders"`
	BuyOrders             int                          `json:"buy_orders"`
	SellOrders            int                          `json:"sell_orders"`
	OpenOrderNotionalISK  float64                      `json:"open_order_notional_isk"`
	TransactionsAvailable bool                         `json:"transactions_available"`
	TransactionCount      int                          `json:"transactions_count"`
	TxnWindowDays         int                          `json:"txn_window_days"`
	BuyFlowISK            float64                      `json:"buy_flow_isk"`
	SellFlowISK           float64                      `json:"sell_flow_isk"`
	NetFlowISK            float64                      `json:"net_flow_isk"`
	TopItems              []stationAIRuntimeItemFlow   `json:"top_items,omitempty"`
	Risk                  *engine.PortfolioRiskSummary `json:"risk,omitempty"`
	Notes                 []string                     `json:"notes,omitempty"`
	FetchedAt             string                       `json:"fetched_at"`
}

type stationAIContextRow struct {
	TypeID       int32   `json:"type_id"`
	TypeName     string  `json:"type_name"`
	StationName  string  `json:"station_name"`
	CTS          float64 `json:"cts"`
	Margin       float64 `json:"margin_percent"`
	DailyProfit  float64 `json:"daily_profit"`
	DailyVolume  float64 `json:"daily_volume"`
	S2BBfSRatio  float64 `json:"s2b_bfs_ratio"`
	Action       string  `json:"action"`
	Reason       string  `json:"reason"`
	Confidence   string  `json:"confidence"`
	HighRisk     bool    `json:"high_risk"`
	ExtremePrice bool    `json:"extreme_price"`
}

type stationAIScanSnapshot struct {
	ScopeMode           string  `json:"scope_mode"`
	SystemName          string  `json:"system_name"`
	RegionID            int32   `json:"region_id"`
	StationID           int64   `json:"station_id"`
	Radius              int     `json:"radius"`
	MinMargin           float64 `json:"min_margin"`
	SalesTaxPercent     float64 `json:"sales_tax_percent"`
	BrokerFee           float64 `json:"broker_fee"`
	SplitTradeFees      bool    `json:"split_trade_fees"`
	BuyBrokerFee        float64 `json:"buy_broker_fee_percent"`
	SellBrokerFee       float64 `json:"sell_broker_fee_percent"`
	BuySalesTaxPercent  float64 `json:"buy_sales_tax_percent"`
	SellSalesTaxPercent float64 `json:"sell_sales_tax_percent"`
	CTSProfile          string  `json:"cts_profile"`
	MinDailyVolume      int64   `json:"min_daily_volume"`
	MinItemProfit       float64 `json:"min_item_profit"`
	MinS2BPerDay        float64 `json:"min_s2b_per_day"`
	MinBfSPerDay        float64 `json:"min_bfs_per_day"`
	AvgPricePeriod      int     `json:"avg_price_period"`
	MinPeriodROI        float64 `json:"min_period_roi"`
	BVSRatioMin         float64 `json:"bvs_ratio_min"`
	BVSRatioMax         float64 `json:"bvs_ratio_max"`
	MaxPVI              float64 `json:"max_pvi"`
	MaxSDS              float64 `json:"max_sds"`
	LimitBuyToPriceLow  bool    `json:"limit_buy_to_price_low"`
	FlagExtremePrices   bool    `json:"flag_extreme_prices"`
	IncludeStructures   bool    `json:"include_structures"`
	StructuresApplied   bool    `json:"structures_applied"`
	StructureCount      int     `json:"structure_count"`
	StructureIDs        []int64 `json:"structure_ids"`
}

type stationAIContextPayload struct {
	TabID          string                   `json:"tab_id"`
	TabTitle       string                   `json:"tab_title"`
	SystemName     string                   `json:"system_name"`
	StationScope   string                   `json:"station_scope"`
	RegionID       int32                    `json:"region_id"`
	StationID      int64                    `json:"station_id"`
	Radius         int                      `json:"radius"`
	MinMargin      float64                  `json:"min_margin"`
	MinDailyVolume int64                    `json:"min_daily_volume"`
	MinItemProfit  float64                  `json:"min_item_profit"`
	ScanSnapshot   stationAIScanSnapshot    `json:"scan_snapshot"`
	Summary        stationAIContextSummary  `json:"summary"`
	Rows           []stationAIContextRow    `json:"rows"`
	Runtime        *stationAIRuntimeContext `json:"runtime,omitempty"`
}

type stationAIHistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type stationAIChatRequestPayload struct {
	Provider      string                    `json:"provider"`
	APIKey        string                    `json:"api_key"`
	Model         string                    `json:"model"`
	PlannerModel  string                    `json:"planner_model"`
	Temperature   float64                   `json:"temperature"`
	MaxTokens     int                       `json:"max_tokens"`
	AssistantName string                    `json:"assistant_name"`
	Locale        string                    `json:"locale"`
	UserMessage   string                    `json:"user_message"`
	EnableWiki    *bool                     `json:"enable_wiki_context"`
	EnableWeb     *bool                     `json:"enable_web_research"`
	EnablePlanner *bool                     `json:"enable_planner"`
	WikiRepo      string                    `json:"wiki_repo"`
	History       []stationAIHistoryMessage `json:"history"`
	Context       stationAIContextPayload   `json:"context"`
}

type stationAIIntentKind string

const (
	stationAIIntentSmallTalk stationAIIntentKind = "smalltalk"
	stationAIIntentTrading   stationAIIntentKind = "trading_analysis"
	stationAIIntentProduct   stationAIIntentKind = "product_help"
	stationAIIntentDebug     stationAIIntentKind = "debug_support"
	stationAIIntentResearch  stationAIIntentKind = "web_research"
	stationAIIntentGeneral   stationAIIntentKind = "general"
)

type stationAIPlannerPlan struct {
	Intent           stationAIIntentKind `json:"intent"`
	ContextLevel     string              `json:"context_level"` // none|summary|full
	ResponseMode     string              `json:"response_mode"` // short|structured|diagnostic|qa
	NeedWiki         bool                `json:"need_wiki"`
	NeedWeb          bool                `json:"need_web"`
	AskClarification bool                `json:"ask_clarification"`
	Clarification    string              `json:"clarification"`
	Agents           []string            `json:"agents"`
}

type stationAIPreflightResult struct {
	Status  string
	Missing []string
	Caveats []string
}

type stationAIProviderReply struct {
	Answer     string
	Model      string
	ProviderID string
	Usage      map[string]interface{}
}

var aiWikiPageCache sync.Map

func (s *Server) getWalletTxnCache(characterID int64) ([]esi.WalletTransaction, bool) {
	s.txnCacheMu.RLock()
	defer s.txnCacheMu.RUnlock()

	if s.txnCache == nil {
		return nil, false
	}
	if s.txnCacheCharacterID != characterID {
		return nil, false
	}
	if time.Since(s.txnCacheTime) >= walletTxnCacheTTL {
		return nil, false
	}

	// Return a copy to avoid accidental sharing across handlers.
	out := make([]esi.WalletTransaction, len(s.txnCache))
	copy(out, s.txnCache)
	return out, true
}

func (s *Server) setWalletTxnCache(characterID int64, txns []esi.WalletTransaction) {
	cached := make([]esi.WalletTransaction, len(txns))
	copy(cached, txns)

	s.txnCacheMu.Lock()
	s.txnCache = cached
	s.txnCacheTime = time.Now()
	s.txnCacheCharacterID = characterID
	s.txnCacheMu.Unlock()
}

func (s *Server) clearWalletTxnCache() {
	s.txnCacheMu.Lock()
	s.txnCache = nil
	s.txnCacheTime = time.Time{}
	s.txnCacheCharacterID = 0
	s.txnCacheMu.Unlock()
}

func (s *Server) getPLEXCache(cacheKey string, maxAge time.Duration) (engine.PLEXDashboard, bool) {
	s.plexCacheMu.RLock()
	defer s.plexCacheMu.RUnlock()

	if s.plexCache == nil || s.plexCacheKey != cacheKey {
		return engine.PLEXDashboard{}, false
	}
	age := time.Since(s.plexCacheTime)
	if age >= maxAge {
		return engine.PLEXDashboard{}, false
	}
	return *s.plexCache, true
}

func (s *Server) setPLEXCache(cacheKey string, dashboard engine.PLEXDashboard) {
	s.plexCacheMu.Lock()
	s.plexCache = &dashboard
	s.plexCacheTime = time.Now()
	s.plexCacheKey = cacheKey
	s.plexCacheMu.Unlock()
}

func cloneConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return config.Default()
	}
	copied := *cfg
	if cfg.SourceRegions != nil {
		copied.SourceRegions = append([]string(nil), cfg.SourceRegions...)
	}
	if cfg.CategoryIDs != nil {
		copied.CategoryIDs = append([]int32(nil), cfg.CategoryIDs...)
	}
	return &copied
}

func secureCookieFromRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	return false
}

func generateUserID() string {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return db.DefaultUserID
	}
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func generateUserCookieSecret() []byte {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return []byte("eveflipper-user-cookie-secret-fallback")
	}
	return secret
}

func loadOrCreateUserCookieSecret(database *db.DB) []byte {
	secret := generateUserCookieSecret()
	if database == nil || database.SqlDB() == nil {
		return secret
	}

	sqlDB := database.SqlDB()
	if _, err := sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS app_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`); err != nil {
		log.Printf("[API] Failed to ensure app_meta table for user cookie secret: %v", err)
		return secret
	}

	var encoded string
	err := sqlDB.QueryRow("SELECT value FROM app_meta WHERE key = ? LIMIT 1", userIDCookieSecretMetaKey).Scan(&encoded)
	switch {
	case err == nil:
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
		if decodeErr == nil && len(decoded) >= 32 {
			return decoded
		}
	case err != sql.ErrNoRows:
		log.Printf("[API] Failed to load user cookie secret from app_meta: %v", err)
		return secret
	}

	encoded = base64.RawURLEncoding.EncodeToString(secret)
	if _, err := sqlDB.Exec(`
		INSERT INTO app_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, userIDCookieSecretMetaKey, encoded); err != nil {
		log.Printf("[API] Failed to persist user cookie secret to app_meta: %v", err)
	}

	return secret
}

func isValidUserID(userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" || len(userID) > 128 {
		return false
	}
	for _, ch := range userID {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return false
	}
	return true
}

func (s *Server) userIDCookieSignature(userID string) []byte {
	secret := s.userIDCookieSecret
	if len(secret) == 0 {
		secret = []byte("eveflipper-user-cookie-secret-fallback")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(userID))
	sum := mac.Sum(nil)
	return sum[:userIDCookieSignatureBytes]
}

func (s *Server) signedUserIDCookieValue(userID string) string {
	signature := base64.RawURLEncoding.EncodeToString(s.userIDCookieSignature(userID))
	return userID + "." + signature
}

func (s *Server) parseSignedUserIDCookieValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	sep := strings.LastIndexByte(value, '.')
	if sep <= 0 || sep >= len(value)-1 {
		return "", false
	}

	userID := strings.TrimSpace(value[:sep])
	signatureValue := strings.TrimSpace(value[sep+1:])
	if !isValidUserID(userID) || signatureValue == "" {
		return "", false
	}

	gotSignature, err := base64.RawURLEncoding.DecodeString(signatureValue)
	if err != nil {
		return "", false
	}
	wantSignature := s.userIDCookieSignature(userID)
	if len(gotSignature) != len(wantSignature) {
		return "", false
	}
	if !hmac.Equal(gotSignature, wantSignature) {
		return "", false
	}
	return userID, true
}

func (s *Server) setUserIDCookie(w http.ResponseWriter, r *http.Request, userID string) string {
	userID = strings.TrimSpace(userID)
	if !isValidUserID(userID) {
		userID = generateUserID()
		if !isValidUserID(userID) {
			userID = db.DefaultUserID
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     userIDCookieName,
		Value:    s.signedUserIDCookieValue(userID),
		Path:     "/",
		HttpOnly: true,
		Secure:   secureCookieFromRequest(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   userIDCookieMaxAge,
		Expires:  time.Now().Add(365 * 24 * time.Hour),
	})
	return userID
}

func (s *Server) ensureRequestUserID(w http.ResponseWriter, r *http.Request) string {
	headerUserID := strings.TrimSpace(r.Header.Get(userIDHeaderName))
	if isValidUserID(headerUserID) {
		// Keep cookie in sync for browser flows; header remains source of truth.
		if c, err := r.Cookie(userIDCookieName); err != nil {
			s.setUserIDCookie(w, r, headerUserID)
		} else if cookieUserID, ok := s.parseSignedUserIDCookieValue(c.Value); !ok || cookieUserID != headerUserID {
			s.setUserIDCookie(w, r, headerUserID)
		}
		return headerUserID
	}

	if c, err := r.Cookie(userIDCookieName); err == nil {
		if userID, ok := s.parseSignedUserIDCookieValue(c.Value); ok {
			return userID
		}
	}

	return s.setUserIDCookie(w, r, generateUserID())
}

func userIDFromRequest(r *http.Request) string {
	if r == nil {
		return db.DefaultUserID
	}
	if v := r.Context().Value(userIDContextKey); v != nil {
		if userID, ok := v.(string); ok {
			userID = strings.TrimSpace(userID)
			if isValidUserID(userID) {
				return userID
			}
		}
	}
	return db.DefaultUserID
}

func (s *Server) userScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := s.ensureRequestUserID(w, r)
		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func normalizeAuthRevisionUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return db.DefaultUserID
	}
	return userID
}

func (s *Server) authRevisionForUser(userID string) int64 {
	userID = normalizeAuthRevisionUserID(userID)
	s.authRevisionMu.Lock()
	defer s.authRevisionMu.Unlock()
	if s.authRevision == nil {
		return 0
	}
	return s.authRevision[userID]
}

func (s *Server) bumpAuthRevision(userID string) int64 {
	userID = normalizeAuthRevisionUserID(userID)
	s.authRevisionMu.Lock()
	defer s.authRevisionMu.Unlock()
	if s.authRevision == nil {
		s.authRevision = make(map[string]int64)
	}
	s.authRevision[userID]++
	return s.authRevision[userID]
}

func (s *Server) loadConfigForUser(userID string) *config.Config {
	if s.db != nil {
		return s.db.LoadConfigForUser(userID)
	}
	return cloneConfig(s.cfg)
}

func (s *Server) saveConfigForUser(userID string, cfg *config.Config) error {
	if s.db != nil {
		return s.db.SaveConfigForUser(userID, cfg)
	}
	s.cfg = cloneConfig(cfg)
	return nil
}

// NewServer creates a Server with the given config, ESI client, and database.
func NewServer(cfg *config.Config, esiClient *esi.Client, database *db.DB, ssoConfig *auth.SSOConfig, sessions *auth.SessionStore) *Server {
	s := &Server{
		cfg:                cfg,
		esi:                esiClient,
		db:                 database,
		sso:                ssoConfig,
		sessions:           sessions,
		wikiRAG:            newStationAIWikiRAG(),
		ssoStates:          make(map[string]ssoStateEntry),
		plexBuildSem:       make(chan struct{}, 1),
		userIDCookieSecret: loadOrCreateUserCookieSecret(database),
		authRevision:       make(map[string]int64),
		appVersion:         "dev",
		appFlavor:          "classic",
		updateHTTP:         &http.Client{Timeout: 45 * time.Second},
		updateSkipByUser:   make(map[string]string),
	}
	if s.wikiRAG != nil {
		s.wikiRAG.Start(defaultStationAIWikiRepo)
	}
	return s
}

func (s *Server) SetAppVersion(v string) {
	v = strings.TrimSpace(v)
	if v == "" {
		v = "dev"
	}
	s.appVersion = v
}

func (s *Server) SetAppFlavor(v string) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		v = "classic"
	}
	s.appFlavor = v
}

// SetSDE is called when SDE data finishes loading.
func (s *Server) SetSDE(data *sde.Data) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sdeData = data
	scanner := engine.NewScanner(data, s.esi)
	scanner.History = s.db
	s.scanner = scanner
	s.industryAnalyzer = engine.NewIndustryAnalyzer(data, s.esi)

	// Initialize demand analyzer with region names from SDE
	s.demandAnalyzer = zkillboard.NewDemandAnalyzer(data.RegionNames())

	// Initialize corporation demo provider
	s.demoCorpProvider = corp.NewDemoCorpProvider()

	// Initialize gank check route analyzer
	s.ganker = gankcheck.NewChecker(zkillboard.NewClient(), s.esi, data, data.Universe)

	s.ready = true
}

func (s *Server) isReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready
}

// Handler returns the HTTP handler with all API routes and CORS middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/update/check", s.handleUpdateCheck)
	mux.HandleFunc("POST /api/update/skip", s.handleUpdateSkipForSession)
	mux.HandleFunc("POST /api/update/apply", s.handleUpdateApply)
	mux.HandleFunc("POST /api/internal/wiki/gollum", s.handleInternalWikiGollumWebhook)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/config", s.handleSetConfig)
	mux.HandleFunc("POST /api/alerts/test", s.handleAlertsTest)
	mux.HandleFunc("GET /api/systems", s.handleGetSystems)
	mux.HandleFunc("GET /api/systems/autocomplete", s.handleAutocomplete)
	mux.HandleFunc("GET /api/regions/autocomplete", s.handleRegionAutocomplete)
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/scan/multi-region", s.handleScanMultiRegion)
	mux.HandleFunc("POST /api/scan/regional-day", s.handleScanRegionalDay)
	mux.HandleFunc("POST /api/scan/contracts", s.handleScanContracts)
	mux.HandleFunc("POST /api/route/find", s.handleRouteFind)
	mux.HandleFunc("GET /api/watchlist", s.handleGetWatchlist)
	mux.HandleFunc("POST /api/watchlist", s.handleAddWatchlist)
	mux.HandleFunc("DELETE /api/watchlist/{typeID}", s.handleDeleteWatchlist)
	mux.HandleFunc("PUT /api/watchlist/{typeID}", s.handleUpdateWatchlist)
	mux.HandleFunc("GET /api/alerts/history", s.handleGetAlertHistory)
	mux.HandleFunc("POST /api/scan/station", s.handleScanStation)
	mux.HandleFunc("GET /api/stations", s.handleGetStations)
	mux.HandleFunc("GET /api/scan/history", s.handleGetHistory)
	mux.HandleFunc("GET /api/scan/history/{id}", s.handleGetHistoryByID)
	mux.HandleFunc("GET /api/scan/history/{id}/results", s.handleGetHistoryResults)
	mux.HandleFunc("DELETE /api/scan/history/{id}", s.handleDeleteHistory)
	mux.HandleFunc("POST /api/scan/history/clear", s.handleClearHistory)
	// Auth
	mux.HandleFunc("GET /api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("GET /api/auth/callback", s.handleAuthCallback)
	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("POST /api/auth/character/select", s.handleAuthCharacterSelect)
	mux.HandleFunc("DELETE /api/auth/characters/{characterID}", s.handleAuthCharacterDelete)
	mux.HandleFunc("GET /api/auth/character", s.handleAuthCharacter)
	mux.HandleFunc("GET /api/auth/location", s.handleAuthLocation)
	mux.HandleFunc("GET /api/auth/undercuts", s.handleAuthUndercuts)
	mux.HandleFunc("GET /api/auth/orders/desk", s.handleAuthOrderDesk)
	mux.HandleFunc("GET /api/auth/station/trade-states", s.handleAuthGetStationTradeStates)
	mux.HandleFunc("POST /api/auth/station/trade-states/set", s.handleAuthSetStationTradeState)
	mux.HandleFunc("POST /api/auth/station/trade-states/delete", s.handleAuthDeleteStationTradeStates)
	mux.HandleFunc("POST /api/auth/station/trade-states/clear", s.handleAuthClearStationTradeStates)
	mux.HandleFunc("POST /api/auth/station/cache/reboot", s.handleAuthRebootStationCache)
	mux.HandleFunc("GET /api/auth/industry/projects", s.handleAuthListIndustryProjects)
	mux.HandleFunc("POST /api/auth/industry/projects", s.handleAuthCreateIndustryProject)
	mux.HandleFunc("GET /api/auth/industry/projects/{projectID}/snapshot", s.handleAuthIndustryProjectSnapshot)
	mux.HandleFunc("POST /api/auth/industry/projects/{projectID}/plan/preview", s.handleAuthPreviewIndustryProjectPlan)
	mux.HandleFunc("POST /api/auth/industry/projects/{projectID}/plan", s.handleAuthPlanIndustryProject)
	mux.HandleFunc("POST /api/auth/industry/projects/{projectID}/materials/rebalance", s.handleAuthRebalanceIndustryProjectMaterials)
	mux.HandleFunc("POST /api/auth/industry/projects/{projectID}/blueprints/sync", s.handleAuthSyncIndustryProjectBlueprintPool)
	mux.HandleFunc("PATCH /api/auth/industry/tasks/status", s.handleAuthUpdateIndustryTaskStatus)
	mux.HandleFunc("PATCH /api/auth/industry/tasks/status/bulk", s.handleAuthBulkUpdateIndustryTaskStatus)
	mux.HandleFunc("PATCH /api/auth/industry/tasks/priority", s.handleAuthUpdateIndustryTaskPriority)
	mux.HandleFunc("PATCH /api/auth/industry/tasks/priority/bulk", s.handleAuthBulkUpdateIndustryTaskPriority)
	mux.HandleFunc("PATCH /api/auth/industry/jobs/status", s.handleAuthUpdateIndustryJobStatus)
	mux.HandleFunc("PATCH /api/auth/industry/jobs/status/bulk", s.handleAuthBulkUpdateIndustryJobStatus)
	mux.HandleFunc("GET /api/auth/industry/ledger", s.handleAuthIndustryLedger)
	mux.HandleFunc("POST /api/auth/station/command", s.handleAuthStationCommand)
	mux.HandleFunc("POST /api/auth/station/ai/chat", s.handleAuthStationAIChat)
	mux.HandleFunc("POST /api/auth/station/ai/chat/stream", s.handleAuthStationAIChatStream)
	mux.HandleFunc("GET /api/auth/portfolio", s.handleAuthPortfolio)
	mux.HandleFunc("GET /api/auth/portfolio/optimize", s.handleAuthPortfolioOptimize)
	mux.HandleFunc("GET /api/auth/structures", s.handleAuthStructures)
	// UI operations (requires auth)
	mux.HandleFunc("POST /api/ui/open-market", s.handleUIOpenMarket)
	mux.HandleFunc("POST /api/ui/set-waypoint", s.handleUISetWaypoint)
	mux.HandleFunc("POST /api/ui/open-contract", s.handleUIOpenContract)
	// Contracts
	mux.HandleFunc("GET /api/contracts/{contract_id}/items", s.handleGetContractItems)
	// Industry
	mux.HandleFunc("POST /api/industry/analyze", s.handleIndustryAnalyze)
	mux.HandleFunc("GET /api/industry/search", s.handleIndustrySearch)
	mux.HandleFunc("GET /api/industry/systems", s.handleIndustrySystems)
	mux.HandleFunc("GET /api/industry/status", s.handleIndustryStatus)
	mux.HandleFunc("POST /api/execution/plan", s.handleExecutionPlan)
	// Demand / War Tracker
	mux.HandleFunc("GET /api/demand/regions", s.handleDemandRegions)
	mux.HandleFunc("GET /api/demand/hotzones", s.handleDemandHotZones)
	mux.HandleFunc("GET /api/demand/region/{regionID}", s.handleDemandRegion)
	mux.HandleFunc("GET /api/demand/opportunities/{regionID}", s.handleDemandOpportunities)
	mux.HandleFunc("GET /api/demand/fittings/{regionID}", s.handleDemandFittings)
	mux.HandleFunc("POST /api/demand/refresh", s.handleDemandRefresh)
	// PLEX+
	mux.HandleFunc("GET /api/plex/dashboard", s.handlePLEXDashboard)
	// Corporation
	mux.HandleFunc("GET /api/auth/roles", s.handleAuthRoles)
	mux.HandleFunc("GET /api/corp/dashboard", s.handleCorpDashboard)
	mux.HandleFunc("GET /api/corp/members", s.handleCorpMembers)
	mux.HandleFunc("GET /api/corp/wallets", s.handleCorpWallets)
	mux.HandleFunc("GET /api/corp/journal", s.handleCorpJournal)
	mux.HandleFunc("GET /api/corp/orders", s.handleCorpOrders)
	mux.HandleFunc("GET /api/corp/industry", s.handleCorpIndustry)
	mux.HandleFunc("GET /api/corp/mining", s.handleCorpMining)
	// Gank Check
	mux.HandleFunc("GET /api/gankcheck", s.handleGankCheck)
	mux.HandleFunc("GET /api/gankcheck/detail", s.handleGankCheckDetail)
	mux.HandleFunc("GET /api/gankcheck/batch", s.handleGankCheckBatch)
	return corsMiddleware(s.userScopeMiddleware(mux))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		allowedOrigin := ""
		if origin != "" && isAllowedCORSOrigin(origin, r.Host) {
			allowedOrigin = origin
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-EveFlipper-UID")
		if r.Method == "OPTIONS" {
			if origin != "" && allowedOrigin == "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAllowedCORSOrigin(origin, requestHost string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	originHost := normalizeHost(u.Host)
	reqHost := normalizeHost(requestHost)
	if originHost == "" || reqHost == "" {
		return false
	}
	if originHost == reqHost {
		return true
	}
	return isLoopbackHost(originHost) && isLoopbackHost(reqHost)
}

func normalizeHost(hostPort string) string {
	if hostPort == "" {
		return ""
	}
	u, err := url.Parse("http://" + hostPort)
	if err != nil {
		return strings.ToLower(strings.Trim(hostPort, "[]"))
	}
	return strings.ToLower(u.Hostname())
}

func isLoopbackHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type stationAIWikiGollumPayload struct {
	Repository struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Pages []struct {
		PageName string `json:"page_name"`
		Title    string `json:"title"`
		Action   string `json:"action"`
		SHA      string `json:"sha"`
	} `json:"pages"`
}

func stationAIWikiWebhookSecret() string {
	return strings.TrimSpace(os.Getenv(stationAIWikiWebhookSecretEnv))
}

func validateGitHubWebhookSignature(body []byte, signatureHeader, secret string) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return true
	}
	signatureHeader = strings.TrimSpace(signatureHeader)
	if !strings.HasPrefix(strings.ToLower(signatureHeader), "sha256=") {
		return false
	}
	signatureHex := strings.TrimSpace(signatureHeader[len("sha256="):])
	if signatureHex == "" {
		return false
	}
	got, err := hex.DecodeString(signatureHex)
	if err != nil || len(got) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	want := mac.Sum(nil)
	return hmac.Equal(got, want)
}

func stationAIWebhookRepoFromPayload(payload stationAIWikiGollumPayload) string {
	repo := strings.TrimSpace(payload.Repository.FullName)
	if repo == "" {
		owner := strings.TrimSpace(payload.Repository.Owner.Login)
		name := strings.TrimSpace(payload.Repository.Name)
		if owner != "" && name != "" {
			repo = owner + "/" + name
		}
	}
	return sanitizeWikiRepo(repo)
}

func clearAIWikiPageCacheForRepo(repo string) {
	repo = sanitizeWikiRepo(repo)
	if strings.TrimSpace(repo) == "" {
		return
	}
	prefix := repo + "|"
	aiWikiPageCache.Range(func(key, value interface{}) bool {
		keyStr, ok := key.(string)
		if ok && strings.HasPrefix(keyStr, prefix) {
			aiWikiPageCache.Delete(keyStr)
		}
		return true
	})
}

func (s *Server) handleInternalWikiGollumWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2_000_000))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read webhook body")
		return
	}

	secret := stationAIWikiWebhookSecret()
	signature := r.Header.Get("X-Hub-Signature-256")
	if !validateGitHubWebhookSignature(body, signature, secret) {
		writeError(w, http.StatusUnauthorized, "invalid webhook signature")
		return
	}

	event := strings.ToLower(strings.TrimSpace(r.Header.Get("X-GitHub-Event")))
	delivery := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))
	if event == "" {
		writeError(w, http.StatusBadRequest, "missing X-GitHub-Event header")
		return
	}
	if event == "ping" {
		writeJSONStatus(w, http.StatusAccepted, map[string]interface{}{
			"ok":       true,
			"accepted": true,
			"event":    event,
			"delivery": delivery,
			"message":  "ping acknowledged",
		})
		return
	}
	if event != "gollum" {
		writeJSONStatus(w, http.StatusAccepted, map[string]interface{}{
			"ok":       true,
			"accepted": true,
			"event":    event,
			"delivery": delivery,
			"ignored":  true,
			"message":  "event ignored (expected gollum)",
		})
		return
	}

	var payload stationAIWikiGollumPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook json")
		return
	}
	repo := stationAIWebhookRepoFromPayload(payload)
	clearAIWikiPageCacheForRepo(repo)

	if s.wikiRAG != nil {
		go func(repo string, pages int, delivery string) {
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), stationAIWikiWebhookRefreshTimeout)
			defer cancel()
			idx, err := s.wikiRAG.ForceRefresh(ctx, repo)
			if err != nil {
				log.Printf(
					"[AI][WIKI-RAG] webhook refresh failed repo=%s pages=%d delivery=%s err=%v",
					repo,
					pages,
					delivery,
					err,
				)
				return
			}
			chunks := 0
			builtAt := ""
			if idx != nil {
				chunks = len(idx.Chunks)
				builtAt = idx.BuiltAt
			}
			log.Printf(
				"[AI][WIKI-RAG] webhook refresh complete repo=%s pages=%d delivery=%s chunks=%d built_at=%s took=%s",
				repo,
				pages,
				delivery,
				chunks,
				builtAt,
				time.Since(start).Round(time.Millisecond),
			)
		}(repo, len(payload.Pages), delivery)
	}

	writeJSONStatus(w, http.StatusAccepted, map[string]interface{}{
		"ok":       true,
		"accepted": true,
		"queued":   s.wikiRAG != nil,
		"event":    event,
		"delivery": delivery,
		"repo":     repo,
		"pages":    len(payload.Pages),
	})
}

// isPlayerStructure returns true if the location ID is a player-owned structure (not NPC station).
func isPlayerStructure(id int64) bool {
	return engine.IsPlayerStructureLocationID(id)
}

func filterFlipResultsExcludeStructures(results []engine.FlipResult) []engine.FlipResult {
	if len(results) == 0 {
		return results
	}
	filtered := results[:0]
	for _, r := range results {
		if isPlayerStructure(r.BuyLocationID) || isPlayerStructure(r.SellLocationID) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func filterFlipResultsMarketDisabled(results []engine.FlipResult) []engine.FlipResult {
	if len(results) == 0 {
		return results
	}
	filtered := results[:0]
	for _, r := range results {
		if engine.IsMarketDisabledTypeID(r.TypeID) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func flipResultKPIProfit(r engine.FlipResult) float64 {
	if r.RealProfit > 0 {
		return r.RealProfit
	}
	if r.ExpectedProfit > 0 {
		return r.ExpectedProfit
	}
	return r.TotalProfit
}

func stationTradeKPIProfit(r engine.StationTrade) float64 {
	if r.DailyProfit != 0 {
		return r.DailyProfit
	}
	if r.RealProfit > 0 {
		return r.RealProfit
	}
	// TotalProfit is full-book notional, not a daily metric — do not use as fallback.
	return 0
}

func contractResultKPIProfit(r engine.ContractResult) float64 {
	if r.ExpectedProfit > 0 {
		return r.ExpectedProfit
	}
	return r.Profit
}

func filterRouteResultsExcludeStructures(results []engine.RouteResult) []engine.RouteResult {
	if len(results) == 0 {
		return results
	}
	filtered := results[:0]
	for _, route := range results {
		skip := false
		for _, hop := range route.Hops {
			if isPlayerStructure(hop.LocationID) || isPlayerStructure(hop.DestLocationID) {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, route)
		}
	}
	return filtered
}

func filterRouteResultsMarketDisabled(results []engine.RouteResult) []engine.RouteResult {
	if len(results) == 0 {
		return results
	}
	filtered := results[:0]
	for _, route := range results {
		blocked := false
		for _, hop := range route.Hops {
			if engine.IsMarketDisabledTypeID(hop.TypeID) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		filtered = append(filtered, route)
	}
	return filtered
}

func filterStationTradesExcludeStructures(results []engine.StationTrade) []engine.StationTrade {
	if len(results) == 0 {
		return results
	}
	filtered := results[:0]
	for _, r := range results {
		if isPlayerStructure(r.StationID) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func filterStationTradesMarketDisabled(results []engine.StationTrade) []engine.StationTrade {
	if len(results) == 0 {
		return results
	}
	filtered := results[:0]
	for _, r := range results {
		if engine.IsMarketDisabledTypeID(r.TypeID) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

type stationCacheMeta struct {
	CurrentRevision int64  `json:"current_revision"`
	LastRefreshAt   string `json:"last_refresh_at,omitempty"`
	NextExpiryAt    string `json:"next_expiry_at,omitempty"`
	MinTTLSec       int64  `json:"min_ttl_sec"`
	MaxTTLSec       int64  `json:"max_ttl_sec"`
	Regions         int    `json:"regions"`
	Entries         int    `json:"entries"`
	Stale           bool   `json:"stale"`
}

func mapRegionIDSet(regionIDs map[int32]bool) []int32 {
	if len(regionIDs) == 0 {
		return nil
	}
	out := make([]int32, 0, len(regionIDs))
	for regionID := range regionIDs {
		out = append(out, regionID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Server) stationCacheMetaForRegions(regionIDs map[int32]bool) stationCacheMeta {
	if s == nil || s.esi == nil {
		return stationCacheMeta{Regions: len(regionIDs)}
	}
	window := s.esi.OrderCacheWindow(mapRegionIDSet(regionIDs), "all")
	meta := stationCacheMeta{
		CurrentRevision: window.CurrentRevision,
		MinTTLSec:       window.MinTTLSeconds,
		MaxTTLSec:       window.MaxTTLSeconds,
		Regions:         window.Regions,
		Entries:         window.Entries,
		Stale:           window.Stale,
	}
	if !window.LastRefreshAt.IsZero() {
		meta.LastRefreshAt = window.LastRefreshAt.UTC().Format(time.RFC3339)
	}
	if !window.NextExpiryAt.IsZero() {
		meta.NextExpiryAt = window.NextExpiryAt.UTC().Format(time.RFC3339)
	}
	return meta
}

func mergeRegionSets(sets ...map[int32]bool) map[int32]bool {
	out := make(map[int32]bool)
	for _, set := range sets {
		for regionID := range set {
			if regionID > 0 {
				out[regionID] = true
			}
		}
	}
	return out
}

func stationCacheMetaFromWindows(regionCount int, windows ...esi.OrderCacheWindow) stationCacheMeta {
	meta := stationCacheMeta{Regions: regionCount}
	now := time.Now()

	var (
		found       bool
		minExpiry   time.Time
		maxExpiry   time.Time
		lastRefresh time.Time
		entries     int
	)

	for _, window := range windows {
		entries += window.Entries
		if window.NextExpiryAt.IsZero() {
			continue
		}
		if !found || window.NextExpiryAt.Before(minExpiry) {
			minExpiry = window.NextExpiryAt
		}
		if !found || window.NextExpiryAt.After(maxExpiry) {
			maxExpiry = window.NextExpiryAt
		}
		if window.LastRefreshAt.After(lastRefresh) {
			lastRefresh = window.LastRefreshAt
		}
		found = true
	}

	meta.Entries = entries
	if !found {
		return meta
	}

	minTTL := int64(time.Until(minExpiry).Seconds())
	maxTTL := int64(time.Until(maxExpiry).Seconds())
	if minTTL < 0 {
		minTTL = 0
	}
	if maxTTL < 0 {
		maxTTL = 0
	}

	meta.CurrentRevision = minExpiry.Unix()
	meta.MinTTLSec = minTTL
	meta.MaxTTLSec = maxTTL
	meta.Stale = now.After(minExpiry)
	if !lastRefresh.IsZero() {
		meta.LastRefreshAt = lastRefresh.UTC().Format(time.RFC3339)
	}
	meta.NextExpiryAt = minExpiry.UTC().Format(time.RFC3339)
	return meta
}

// filterContractResultsMarketDisabled is a defense-in-depth guard:
// even if upstream scan/history contained unsafe contracts, drop ones that include
// market-disabled types (e.g. MPTC) before returning to UI.
func (s *Server) filterContractResultsMarketDisabled(results []engine.ContractResult) []engine.ContractResult {
	if len(results) == 0 {
		return results
	}
	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()
	if scanner == nil {
		return results
	}

	contractIDs := make([]int32, 0, len(results))
	for _, r := range results {
		if r.ContractID > 0 {
			contractIDs = append(contractIDs, r.ContractID)
		}
	}
	if len(contractIDs) == 0 {
		return results
	}

	// Prefer fail-closed for this risk class: if contract items cannot be verified,
	// do not surface the result to avoid ghost-market losses.
	itemsByContract := s.esi.FetchContractItemsBatch(contractIDs, scanner.ContractItemsCache, func(done, total int) {})
	filtered := results[:0]
	dropped := 0

	for _, r := range results {
		items, ok := itemsByContract[r.ContractID]
		if !ok {
			dropped++
			continue
		}

		blocked := false
		for _, item := range items {
			if item.Quantity > 0 && engine.IsMarketDisabledTypeID(item.TypeID) {
				blocked = true
				break
			}
		}
		if blocked {
			dropped++
			continue
		}
		filtered = append(filtered, r)
	}

	if dropped > 0 {
		log.Printf("[API] Contracts post-filter: dropped %d/%d results (market-disabled types or unverifiable items)", dropped, len(results))
	}
	return filtered
}

// enrichStructureNames resolves player-structure names in FlipResult slice
// if the user is authenticated. Results with unresolved structure names are
// filtered out (user can't find unnamed structures in-game).
func (s *Server) enrichStructureNames(userID string, results []engine.FlipResult) []engine.FlipResult {
	if s.sessions == nil {
		return results
	}
	token, err := s.sessions.EnsureValidTokenForUser(s.sso, userID)
	if err != nil {
		return results // not authenticated, skip
	}
	structureIDs := make(map[int64]bool)
	for _, r := range results {
		if isPlayerStructure(r.BuyLocationID) {
			structureIDs[r.BuyLocationID] = true
		}
		if isPlayerStructure(r.SellLocationID) {
			structureIDs[r.SellLocationID] = true
		}
	}
	if len(structureIDs) == 0 {
		return results
	}
	s.esi.PrefetchStructureNames(structureIDs, token)

	// Resolve names and track which IDs remain unresolved
	resolved := make(map[int64]string)
	unresolved := make(map[int64]bool)
	for id := range structureIDs {
		name := s.esi.StationName(id)
		if strings.HasPrefix(name, "Structure ") || strings.HasPrefix(name, "Location ") {
			unresolved[id] = true
		} else {
			resolved[id] = name
		}
	}

	// Update names and filter out results with unresolved structures
	filtered := make([]engine.FlipResult, 0, len(results))
	for i := range results {
		if unresolved[results[i].BuyLocationID] || unresolved[results[i].SellLocationID] {
			continue // skip — user can't find this structure in-game
		}
		if name, ok := resolved[results[i].BuyLocationID]; ok {
			results[i].BuyStation = name
		}
		if name, ok := resolved[results[i].SellLocationID]; ok {
			results[i].SellStation = name
		}
		filtered = append(filtered, results[i])
	}
	if dropped := len(results) - len(filtered); dropped > 0 {
		log.Printf("[API] Filtered %d results with unresolved structure names", dropped)
	}
	return filtered
}

// enrichRouteStructureNames resolves player-structure names in RouteResult slice.
// Routes containing hops with unresolved structure names are filtered out.
func (s *Server) enrichRouteStructureNames(userID string, results []engine.RouteResult) []engine.RouteResult {
	if s.sessions == nil {
		return results
	}
	token, err := s.sessions.EnsureValidTokenForUser(s.sso, userID)
	if err != nil {
		return results
	}
	structureIDs := make(map[int64]bool)
	for _, route := range results {
		for _, hop := range route.Hops {
			if isPlayerStructure(hop.LocationID) {
				structureIDs[hop.LocationID] = true
			}
			if isPlayerStructure(hop.DestLocationID) {
				structureIDs[hop.DestLocationID] = true
			}
		}
	}
	if len(structureIDs) == 0 {
		return results
	}
	s.esi.PrefetchStructureNames(structureIDs, token)

	// Resolve names and track which IDs remain unresolved
	resolved := make(map[int64]string)
	unresolved := make(map[int64]bool)
	for id := range structureIDs {
		name := s.esi.StationName(id)
		if strings.HasPrefix(name, "Structure ") || strings.HasPrefix(name, "Location ") {
			unresolved[id] = true
		} else {
			resolved[id] = name
		}
	}

	// Update names and filter out routes with unresolved structures
	filtered := make([]engine.RouteResult, 0, len(results))
	for i := range results {
		skip := false
		for j := range results[i].Hops {
			if unresolved[results[i].Hops[j].LocationID] || unresolved[results[i].Hops[j].DestLocationID] {
				skip = true
				break
			}
			if name, ok := resolved[results[i].Hops[j].LocationID]; ok {
				results[i].Hops[j].StationName = name
			}
			if name, ok := resolved[results[i].Hops[j].DestLocationID]; ok {
				results[i].Hops[j].DestStationName = name
			}
		}
		if !skip {
			filtered = append(filtered, results[i])
		}
	}
	if dropped := len(results) - len(filtered); dropped > 0 {
		log.Printf("[API] Filtered %d routes with unresolved structure names", dropped)
	}
	return filtered
}

// --- Handlers ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sdeLoaded := s.ready
	var systemCount, typeCount int
	if s.sdeData != nil {
		systemCount = len(s.sdeData.Systems)
		typeCount = len(s.sdeData.Types)
	}
	s.mu.RUnlock()

	esiOK := s.esi.HealthCheck()
	_, lastOK := s.esi.HealthStatus()

	result := map[string]interface{}{
		"sde_loaded":  sdeLoaded,
		"sde_systems": systemCount,
		"sde_types":   typeCount,
		"esi_ok":      esiOK,
	}

	// Add last successful ESI connection time if available
	if !lastOK.IsZero() {
		result["esi_last_ok"] = lastOK.Unix()
	}

	writeJSON(w, result)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	cfg := s.loadConfigForUser(userID)
	writeJSON(w, cfg)
}

func (s *Server) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	cfg := s.loadConfigForUser(userID)

	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	if v, ok := patch["system_name"]; ok {
		json.Unmarshal(v, &cfg.SystemName)
	}
	if v, ok := patch["ignored_system_ids"]; ok {
		json.Unmarshal(v, &cfg.IgnoredSystemIDs)
	}
	if v, ok := patch["cargo_capacity"]; ok {
		json.Unmarshal(v, &cfg.CargoCapacity)
	}
	if v, ok := patch["buy_radius"]; ok {
		json.Unmarshal(v, &cfg.BuyRadius)
	}
	if v, ok := patch["sell_radius"]; ok {
		json.Unmarshal(v, &cfg.SellRadius)
	}
	if v, ok := patch["min_margin"]; ok {
		json.Unmarshal(v, &cfg.MinMargin)
	}
	if v, ok := patch["sales_tax_percent"]; ok {
		json.Unmarshal(v, &cfg.SalesTaxPercent)
	}
	if v, ok := patch["broker_fee_percent"]; ok {
		json.Unmarshal(v, &cfg.BrokerFeePercent)
	}
	if v, ok := patch["split_trade_fees"]; ok {
		json.Unmarshal(v, &cfg.SplitTradeFees)
	}
	if v, ok := patch["buy_broker_fee_percent"]; ok {
		json.Unmarshal(v, &cfg.BuyBrokerFeePercent)
	}
	if v, ok := patch["sell_broker_fee_percent"]; ok {
		json.Unmarshal(v, &cfg.SellBrokerFeePercent)
	}
	if v, ok := patch["buy_sales_tax_percent"]; ok {
		json.Unmarshal(v, &cfg.BuySalesTaxPercent)
	}
	if v, ok := patch["sell_sales_tax_percent"]; ok {
		json.Unmarshal(v, &cfg.SellSalesTaxPercent)
	}
	if v, ok := patch["min_daily_volume"]; ok {
		json.Unmarshal(v, &cfg.MinDailyVolume)
	}
	if v, ok := patch["max_investment"]; ok {
		json.Unmarshal(v, &cfg.MaxInvestment)
	}
	if v, ok := patch["min_item_profit"]; ok {
		json.Unmarshal(v, &cfg.MinItemProfit)
	}
	if v, ok := patch["min_s2b_per_day"]; ok {
		json.Unmarshal(v, &cfg.MinS2BPerDay)
	}
	if v, ok := patch["min_bfs_per_day"]; ok {
		json.Unmarshal(v, &cfg.MinBfSPerDay)
	}
	if v, ok := patch["min_s2b_bfs_ratio"]; ok {
		json.Unmarshal(v, &cfg.MinS2BBfSRatio)
	}
	if v, ok := patch["max_s2b_bfs_ratio"]; ok {
		json.Unmarshal(v, &cfg.MaxS2BBfSRatio)
	}
	if v, ok := patch["min_route_security"]; ok {
		json.Unmarshal(v, &cfg.MinRouteSecurity)
	}
	if v, ok := patch["avg_price_period"]; ok {
		json.Unmarshal(v, &cfg.AvgPricePeriod)
	}
	if v, ok := patch["min_period_roi"]; ok {
		json.Unmarshal(v, &cfg.MinPeriodROI)
	}
	if v, ok := patch["max_dos"]; ok {
		json.Unmarshal(v, &cfg.MaxDOS)
	}
	if v, ok := patch["min_demand_per_day"]; ok {
		json.Unmarshal(v, &cfg.MinDemandPerDay)
	}
	if v, ok := patch["purchase_demand_days"]; ok {
		json.Unmarshal(v, &cfg.PurchaseDemandDays)
	}
	if v, ok := patch["shipping_cost_per_m3_jump"]; ok {
		json.Unmarshal(v, &cfg.ShippingCostPerM3Jump)
	}
	if v, ok := patch["source_regions"]; ok {
		json.Unmarshal(v, &cfg.SourceRegions)
	}
	if v, ok := patch["target_region"]; ok {
		json.Unmarshal(v, &cfg.TargetRegion)
	}
	if v, ok := patch["target_market_system"]; ok {
		json.Unmarshal(v, &cfg.TargetMarketSystem)
	}
	if v, ok := patch["target_market_location_id"]; ok {
		json.Unmarshal(v, &cfg.TargetMarketLocationID)
	}
	if v, ok := patch["category_ids"]; ok {
		json.Unmarshal(v, &cfg.CategoryIDs)
	}
	if v, ok := patch["sell_order_mode"]; ok {
		json.Unmarshal(v, &cfg.SellOrderMode)
	}
	if v, ok := patch["alert_telegram"]; ok {
		json.Unmarshal(v, &cfg.AlertTelegram)
	}
	if v, ok := patch["alert_discord"]; ok {
		json.Unmarshal(v, &cfg.AlertDiscord)
	}
	if v, ok := patch["alert_desktop"]; ok {
		json.Unmarshal(v, &cfg.AlertDesktop)
	}
	if v, ok := patch["alert_telegram_token"]; ok {
		json.Unmarshal(v, &cfg.AlertTelegramToken)
	}
	if v, ok := patch["alert_telegram_chat_id"]; ok {
		json.Unmarshal(v, &cfg.AlertTelegramChatID)
	}
	if v, ok := patch["alert_discord_webhook"]; ok {
		json.Unmarshal(v, &cfg.AlertDiscordWebhook)
	}
	if v, ok := patch["opacity"]; ok {
		json.Unmarshal(v, &cfg.Opacity)
	}
	if len(cfg.IgnoredSystemIDs) > 0 {
		s.mu.RLock()
		var systems map[int32]*sde.SolarSystem
		if s.sdeData != nil {
			systems = s.sdeData.Systems
		}
		s.mu.RUnlock()
		if len(systems) > 0 {
			cfg.IgnoredSystemIDs = normalizeIgnoredSystemIDs(systems, cfg.IgnoredSystemIDs)
		}
	}

	// Validate bounds
	if cfg.CargoCapacity < 0 {
		cfg.CargoCapacity = 0
	}
	if cfg.BuyRadius < 0 {
		cfg.BuyRadius = 0
	}
	if cfg.SellRadius < 0 {
		cfg.SellRadius = 0
	}
	if cfg.MinMargin < 0 {
		cfg.MinMargin = 0
	} else if cfg.MinMargin > 100 {
		cfg.MinMargin = 100
	}
	if cfg.SalesTaxPercent < 0 {
		cfg.SalesTaxPercent = 0
	} else if cfg.SalesTaxPercent > 100 {
		cfg.SalesTaxPercent = 100
	}
	if cfg.BrokerFeePercent < 0 {
		cfg.BrokerFeePercent = 0
	} else if cfg.BrokerFeePercent > 100 {
		cfg.BrokerFeePercent = 100
	}
	if cfg.BuyBrokerFeePercent < 0 {
		cfg.BuyBrokerFeePercent = 0
	} else if cfg.BuyBrokerFeePercent > 100 {
		cfg.BuyBrokerFeePercent = 100
	}
	if cfg.SellBrokerFeePercent < 0 {
		cfg.SellBrokerFeePercent = 0
	} else if cfg.SellBrokerFeePercent > 100 {
		cfg.SellBrokerFeePercent = 100
	}
	if cfg.BuySalesTaxPercent < 0 {
		cfg.BuySalesTaxPercent = 0
	} else if cfg.BuySalesTaxPercent > 100 {
		cfg.BuySalesTaxPercent = 100
	}
	if cfg.SellSalesTaxPercent < 0 {
		cfg.SellSalesTaxPercent = 0
	} else if cfg.SellSalesTaxPercent > 100 {
		cfg.SellSalesTaxPercent = 100
	}
	if cfg.MinDailyVolume < 0 {
		cfg.MinDailyVolume = 0
	}
	if cfg.MaxInvestment < 0 {
		cfg.MaxInvestment = 0
	}
	if cfg.MinItemProfit < 0 {
		cfg.MinItemProfit = 0
	}
	if cfg.MinS2BPerDay < 0 {
		cfg.MinS2BPerDay = 0
	}
	if cfg.MinBfSPerDay < 0 {
		cfg.MinBfSPerDay = 0
	}
	if cfg.MinS2BBfSRatio < 0 {
		cfg.MinS2BBfSRatio = 0
	}
	if cfg.MaxS2BBfSRatio < 0 {
		cfg.MaxS2BBfSRatio = 0
	}
	if cfg.MinRouteSecurity < 0 {
		cfg.MinRouteSecurity = 0
	} else if cfg.MinRouteSecurity > 1 {
		cfg.MinRouteSecurity = 1
	}
	if cfg.AvgPricePeriod <= 0 {
		cfg.AvgPricePeriod = 14
	} else if cfg.AvgPricePeriod > 365 {
		cfg.AvgPricePeriod = 365
	}
	if cfg.MinPeriodROI < 0 {
		cfg.MinPeriodROI = 0
	}
	if cfg.MaxDOS < 0 {
		cfg.MaxDOS = 0
	}
	if cfg.MinDemandPerDay < 0 {
		cfg.MinDemandPerDay = 0
	}
	if cfg.PurchaseDemandDays < 0 {
		cfg.PurchaseDemandDays = 0
	}
	if cfg.ShippingCostPerM3Jump < 0 {
		cfg.ShippingCostPerM3Jump = 0
	}
	if cfg.TargetMarketLocationID < 0 {
		cfg.TargetMarketLocationID = 0
	}
	cfg.TargetRegion = strings.TrimSpace(cfg.TargetRegion)
	cfg.TargetMarketSystem = strings.TrimSpace(cfg.TargetMarketSystem)
	{
		clean := make([]string, 0, len(cfg.SourceRegions))
		seen := make(map[string]bool, len(cfg.SourceRegions))
		for _, name := range cfg.SourceRegions {
			trimmed := strings.TrimSpace(name)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if seen[key] {
				continue
			}
			seen[key] = true
			clean = append(clean, trimmed)
			if len(clean) >= 32 {
				break
			}
		}
		cfg.SourceRegions = clean
	}
	{
		clean := make([]int32, 0, len(cfg.CategoryIDs))
		seen := make(map[int32]bool, len(cfg.CategoryIDs))
		for _, id := range cfg.CategoryIDs {
			if id <= 0 || seen[id] {
				continue
			}
			seen[id] = true
			clean = append(clean, id)
			if len(clean) >= 64 {
				break
			}
		}
		cfg.CategoryIDs = clean
	}
	if cfg.Opacity < 0 {
		cfg.Opacity = 0
	} else if cfg.Opacity > 100 {
		cfg.Opacity = 100
	}
	// Keep at least one alert channel enabled.
	if !cfg.AlertTelegram && !cfg.AlertDiscord && !cfg.AlertDesktop {
		cfg.AlertDesktop = true
	}

	if err := s.saveConfigForUser(userID, cfg); err != nil {
		writeError(w, 500, "failed to save config")
		return
	}
	writeJSON(w, cfg)
}

type alertSendResult struct {
	Sent   []string          `json:"sent"`
	Failed map[string]string `json:"failed,omitempty"`
}

func (s *Server) handleAlertsTest(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	cfg := s.loadConfigForUser(userID)

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, 400, "invalid json")
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		msg = fmt.Sprintf("EVE Flipper test alert (%s)", time.Now().Format(time.RFC3339))
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}

	res := s.sendConfiguredExternalAlerts(cfg, msg)
	writeJSON(w, res)
}

func (s *Server) sendConfiguredExternalAlerts(cfg *config.Config, message string) alertSendResult {
	out := alertSendResult{
		Sent:   []string{},
		Failed: map[string]string{},
	}
	if cfg == nil {
		out.Failed["config"] = "config is not loaded"
		return out
	}

	if cfg.AlertTelegram {
		if strings.TrimSpace(cfg.AlertTelegramToken) == "" || strings.TrimSpace(cfg.AlertTelegramChatID) == "" {
			out.Failed["telegram"] = "telegram token/chat_id not configured"
		} else if err := sendTelegramAlert(cfg.AlertTelegramToken, cfg.AlertTelegramChatID, message); err != nil {
			out.Failed["telegram"] = err.Error()
		} else {
			out.Sent = append(out.Sent, "telegram")
		}
	}
	if cfg.AlertDiscord {
		if strings.TrimSpace(cfg.AlertDiscordWebhook) == "" {
			out.Failed["discord"] = "discord webhook not configured"
		} else if err := sendDiscordAlert(cfg.AlertDiscordWebhook, message); err != nil {
			out.Failed["discord"] = err.Error()
		} else {
			out.Sent = append(out.Sent, "discord")
		}
	}
	if len(out.Failed) == 0 {
		out.Failed = nil
	}
	return out
}

func sendTelegramAlert(token, chatID, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", strings.TrimSpace(token))
	body, _ := json.Marshal(map[string]any{
		"chat_id":                  strings.TrimSpace(chatID),
		"text":                     message,
		"disable_web_page_preview": true,
	})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func sendDiscordAlert(webhookURL, message string) error {
	body, _ := json.Marshal(map[string]any{
		"content": message,
	})
	req, err := http.NewRequest(http.MethodPost, strings.TrimSpace(webhookURL), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Discord webhook usually returns 204 No Content.
	if resp.StatusCode != http.StatusNoContent && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("discord http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (s *Server) handleGetSystems(w http.ResponseWriter, r *http.Request) {
	type systemInfo struct {
		ID       int32   `json:"id"`
		Name     string  `json:"name"`
		Security float64 `json:"security"`
		RegionID int32   `json:"region_id"`
	}
	type systemsResponse struct {
		Systems []systemInfo `json:"systems"`
	}

	if !s.isReady() {
		writeJSON(w, systemsResponse{Systems: []systemInfo{}})
		return
	}

	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}

	s.mu.RLock()
	systems := s.sdeData.Systems
	s.mu.RUnlock()

	rows := make([]systemInfo, 0, len(systems))
	for _, sys := range systems {
		if sys == nil {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(sys.Name), query) {
			continue
		}
		rows = append(rows, systemInfo{
			ID:       sys.ID,
			Name:     sys.Name,
			Security: sys.Security,
			RegionID: sys.RegionID,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name == rows[j].Name {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].Name < rows[j].Name
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}

	writeJSON(w, systemsResponse{Systems: rows})
}

func (s *Server) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if q == "" || !s.isReady() {
		writeJSON(w, map[string][]string{"systems": {}})
		return
	}

	s.mu.RLock()
	names := s.sdeData.SystemNames
	s.mu.RUnlock()

	var prefix, contains []string
	for _, name := range names {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, q) {
			prefix = append(prefix, name)
		} else if strings.Contains(lower, q) {
			contains = append(contains, name)
		}
	}

	result := append(prefix, contains...)
	if len(result) > 15 {
		result = result[:15]
	}

	writeJSON(w, map[string][]string{"systems": result})
}

func (s *Server) handleRegionAutocomplete(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if q == "" || !s.isReady() {
		writeJSON(w, map[string][]string{"regions": {}})
		return
	}

	s.mu.RLock()
	regions := s.sdeData.Regions
	systems := s.sdeData.Systems
	s.mu.RUnlock()

	seen := map[string]bool{}
	var prefix, contains, bySystem []string
	for _, region := range regions {
		lower := strings.ToLower(region.Name)
		if strings.HasPrefix(lower, q) {
			prefix = append(prefix, region.Name)
			seen[region.Name] = true
		} else if strings.Contains(lower, q) {
			contains = append(contains, region.Name)
			seen[region.Name] = true
		}
	}

	// Also match by system name → suggest the region that system belongs to
	for _, sys := range systems {
		if strings.HasPrefix(strings.ToLower(sys.Name), q) {
			if r, ok := regions[sys.RegionID]; ok && !seen[r.Name] {
				bySystem = append(bySystem, r.Name+" ("+sys.Name+")")
				seen[r.Name] = true
			}
		}
	}

	result := append(prefix, contains...)
	result = append(result, bySystem...)
	if len(result) > 15 {
		result = result[:15]
	}

	writeJSON(w, map[string][]string{"regions": result})
}

func normalizeIgnoredSystemIDs(systems map[int32]*sde.SolarSystem, ids []int32) []int32 {
	if len(ids) == 0 || len(systems) == 0 {
		return nil
	}
	seen := make(map[int32]bool, len(ids))
	out := make([]int32, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := systems[id]; !ok {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func ignoredSystemSet(systems map[int32]*sde.SolarSystem, ids []int32) map[int32]bool {
	normalized := normalizeIgnoredSystemIDs(systems, ids)
	if len(normalized) == 0 {
		return nil
	}
	out := make(map[int32]bool, len(normalized))
	for _, id := range normalized {
		out[id] = true
	}
	return out
}

type scanRequest struct {
	SystemName           string  `json:"system_name"`
	IgnoredSystemIDs     []int32 `json:"ignored_system_ids"`
	CargoCapacity        float64 `json:"cargo_capacity"`
	BuyRadius            int     `json:"buy_radius"`
	SellRadius           int     `json:"sell_radius"`
	MinMargin            float64 `json:"min_margin"`
	SalesTaxPercent      float64 `json:"sales_tax_percent"`
	BrokerFeePercent     float64 `json:"broker_fee_percent"` // 0 = instant trades (no broker fee); >0 = applied to both sides
	SplitTradeFees       bool    `json:"split_trade_fees"`
	BuyBrokerFeePercent  float64 `json:"buy_broker_fee_percent"`
	SellBrokerFeePercent float64 `json:"sell_broker_fee_percent"`
	BuySalesTaxPercent   float64 `json:"buy_sales_tax_percent"`
	SellSalesTaxPercent  float64 `json:"sell_sales_tax_percent"`
	// Advanced filters
	MinDailyVolume         int64    `json:"min_daily_volume"`
	MaxInvestment          float64  `json:"max_investment"`
	MinItemProfit          float64  `json:"min_item_profit"`
	MinPeriodROI           float64  `json:"min_period_roi"`
	MaxDOS                 float64  `json:"max_dos"`
	MinDemandPerDay        float64  `json:"min_demand_per_day"`
	PurchaseDemandDays     float64  `json:"purchase_demand_days"`
	MinS2BPerDay           float64  `json:"min_s2b_per_day"`
	MinBfSPerDay           float64  `json:"min_bfs_per_day"`
	MinS2BBfSRatio         float64  `json:"min_s2b_bfs_ratio"`
	MaxS2BBfSRatio         float64  `json:"max_s2b_bfs_ratio"`
	AvgPricePeriod         int      `json:"avg_price_period"`
	ShippingCostPerM3Jump  float64  `json:"shipping_cost_per_m3_jump"`
	MinRouteSecurity       float64  `json:"min_route_security"`        // 0 = all; 0.45 = highsec only; 0.7 = min 0.7
	SourceRegions          []string `json:"source_regions"`            // Optional source region names (e.g. ["The Forge","Domain"]).
	TargetRegion           string   `json:"target_region"`             // Empty = search all by radius; region name = search only in that region
	TargetMarketSystem     string   `json:"target_market_system"`      // Optional destination marketplace system.
	TargetMarketLocationID int64    `json:"target_market_location_id"` // Optional destination marketplace location_id.
	// Contract-specific filters
	MinContractPrice           float64 `json:"min_contract_price"`
	MaxContractMargin          float64 `json:"max_contract_margin"`
	MinPricedRatio             float64 `json:"min_priced_ratio"`
	RequireHistory             bool    `json:"require_history"`
	ContractInstantLiquidation bool    `json:"contract_instant_liquidation"`
	ContractHoldDays           int     `json:"contract_hold_days"`
	ContractTargetConfidence   float64 `json:"contract_target_confidence"`
	ExcludeRigsWithShip        bool    `json:"exclude_rigs_with_ship"`
	// Category filter for regional day trader (empty = all categories)
	CategoryIDs []int32 `json:"category_ids"`
	// Sell-order mode: use target lowest sell price instead of highest buy order price
	SellOrderMode bool `json:"sell_order_mode"`
	// Player structures
	IncludeStructures bool `json:"include_structures"`
}

func (s *Server) parseScanParams(req scanRequest) (engine.ScanParams, error) {
	if !s.isReady() {
		return engine.ScanParams{}, fmt.Errorf("SDE not loaded yet")
	}

	s.mu.RLock()
	systemID, ok := s.sdeData.SystemByName[strings.ToLower(req.SystemName)]
	ignoredSystemIDs := normalizeIgnoredSystemIDs(s.sdeData.Systems, req.IgnoredSystemIDs)

	// Parse target region if specified.
	var targetRegionID int32
	var targetMarketSystemID int32
	sourceRegionIDs := make([]int32, 0, len(req.SourceRegions))
	sourceRegionSeen := make(map[int32]bool, len(req.SourceRegions))
	for _, sourceRegionName := range req.SourceRegions {
		name := strings.TrimSpace(sourceRegionName)
		if name == "" {
			continue
		}
		rid, regionOK := s.sdeData.RegionByName[strings.ToLower(name)]
		if !regionOK {
			s.mu.RUnlock()
			return engine.ScanParams{}, fmt.Errorf("source region not found: %s", sourceRegionName)
		}
		if !sourceRegionSeen[rid] {
			sourceRegionSeen[rid] = true
			sourceRegionIDs = append(sourceRegionIDs, rid)
		}
	}
	targetRegionName := strings.TrimSpace(req.TargetRegion)
	if targetRegionName != "" {
		rid, regionOK := s.sdeData.RegionByName[strings.ToLower(targetRegionName)]
		if regionOK {
			targetRegionID = rid
		} else {
			s.mu.RUnlock()
			return engine.ScanParams{}, fmt.Errorf("region not found: %s", req.TargetRegion)
		}
	}
	if strings.TrimSpace(req.TargetMarketSystem) != "" {
		sid, systemOK := s.sdeData.SystemByName[strings.ToLower(strings.TrimSpace(req.TargetMarketSystem))]
		if !systemOK {
			s.mu.RUnlock()
			return engine.ScanParams{}, fmt.Errorf("target market system not found: %s", req.TargetMarketSystem)
		}
		targetMarketSystemID = sid
		// A concrete destination marketplace system implies its region.
		// Force region scope to that region to avoid conflicting combinations.
		if sys, okSys := s.sdeData.Systems[targetMarketSystemID]; okSys {
			targetRegionID = sys.RegionID
		}
	}
	s.mu.RUnlock()

	if !ok {
		return engine.ScanParams{}, fmt.Errorf("system not found: %s", req.SystemName)
	}

	return engine.ScanParams{
		CurrentSystemID:            systemID,
		IgnoredSystemIDs:           ignoredSystemIDs,
		CargoCapacity:              req.CargoCapacity,
		BuyRadius:                  req.BuyRadius,
		SellRadius:                 req.SellRadius,
		MinMargin:                  req.MinMargin,
		SalesTaxPercent:            req.SalesTaxPercent,
		BrokerFeePercent:           req.BrokerFeePercent,
		SplitTradeFees:             req.SplitTradeFees,
		BuyBrokerFeePercent:        req.BuyBrokerFeePercent,
		SellBrokerFeePercent:       req.SellBrokerFeePercent,
		BuySalesTaxPercent:         req.BuySalesTaxPercent,
		SellSalesTaxPercent:        req.SellSalesTaxPercent,
		MinDailyVolume:             req.MinDailyVolume,
		MaxInvestment:              req.MaxInvestment,
		MinItemProfit:              req.MinItemProfit,
		MinPeriodROI:               req.MinPeriodROI,
		MaxDOS:                     req.MaxDOS,
		MinDemandPerDay:            req.MinDemandPerDay,
		PurchaseDemandDays:         req.PurchaseDemandDays,
		MinS2BPerDay:               req.MinS2BPerDay,
		MinBfSPerDay:               req.MinBfSPerDay,
		MinS2BBfSRatio:             req.MinS2BBfSRatio,
		MaxS2BBfSRatio:             req.MaxS2BBfSRatio,
		AvgPricePeriod:             req.AvgPricePeriod,
		ShippingCostPerM3Jump:      req.ShippingCostPerM3Jump,
		SourceRegionIDs:            sourceRegionIDs,
		TargetMarketSystemID:       targetMarketSystemID,
		TargetMarketLocationID:     req.TargetMarketLocationID,
		MinRouteSecurity:           req.MinRouteSecurity,
		TargetRegionID:             targetRegionID,
		MinContractPrice:           req.MinContractPrice,
		MaxContractMargin:          req.MaxContractMargin,
		MinPricedRatio:             req.MinPricedRatio,
		RequireHistory:             req.RequireHistory,
		ContractInstantLiquidation: req.ContractInstantLiquidation,
		ContractHoldDays:           req.ContractHoldDays,
		ContractTargetConfidence:   req.ContractTargetConfidence,
		ExcludeRigsWithShip:        req.ExcludeRigsWithShip,
		CategoryIDs:                req.CategoryIDs,
		SellOrderMode:              req.SellOrderMode,
		IncludeStructures:          req.IncludeStructures,
	}, nil
}

func mergeRegionSet(dst, src map[int32]bool) {
	if dst == nil || len(src) == 0 {
		return
	}
	for regionID := range src {
		dst[regionID] = true
	}
}

func mergeRegionIDs(dst map[int32]bool, ids []int32) {
	if dst == nil || len(ids) == 0 {
		return
	}
	for _, regionID := range ids {
		if regionID > 0 {
			dst[regionID] = true
		}
	}
}

func (s *Server) regionsWithinRadius(systemID int32, radius int, minSec float64) map[int32]bool {
	out := make(map[int32]bool)
	if systemID == 0 {
		return out
	}
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()
	if sdeData == nil || sdeData.Universe == nil {
		return out
	}

	var systems map[int32]int
	if minSec > 0 {
		systems = sdeData.Universe.SystemsWithinRadiusMinSecurity(systemID, radius, minSec)
	} else {
		systems = sdeData.Universe.SystemsWithinRadius(systemID, radius)
	}
	regions := sdeData.Universe.RegionsInSet(systems)
	for regionID := range regions {
		out[regionID] = true
	}
	return out
}

func (s *Server) regionScopeForFlipScan(params engine.ScanParams, multiRegion bool) map[int32]bool {
	out := make(map[int32]bool)
	// SourceRegionIDs are a regional-trade concept; radius scan keeps classic buy-radius scope.
	if multiRegion && len(params.SourceRegionIDs) > 0 {
		mergeRegionIDs(out, params.SourceRegionIDs)
	} else {
		mergeRegionSet(out, s.regionsWithinRadius(params.CurrentSystemID, params.BuyRadius, params.MinRouteSecurity))
	}
	if multiRegion && params.TargetRegionID > 0 {
		out[params.TargetRegionID] = true
		return out
	}
	mergeRegionSet(out, s.regionsWithinRadius(params.CurrentSystemID, params.SellRadius, params.MinRouteSecurity))
	return out
}

func (s *Server) flipScanRegionScopes(params engine.ScanParams, multiRegion bool) (map[int32]bool, map[int32]bool) {
	buyRegions := make(map[int32]bool)
	sellRegions := make(map[int32]bool)

	// Source regions are used only in multi-region/regional-day flows.
	if multiRegion && len(params.SourceRegionIDs) > 0 {
		mergeRegionIDs(buyRegions, params.SourceRegionIDs)
	} else {
		mergeRegionSet(
			buyRegions,
			s.regionsWithinRadius(params.CurrentSystemID, params.BuyRadius, params.MinRouteSecurity),
		)
	}

	if multiRegion && params.TargetRegionID > 0 {
		sellRegions[params.TargetRegionID] = true
	} else {
		mergeRegionSet(
			sellRegions,
			s.regionsWithinRadius(params.CurrentSystemID, params.SellRadius, params.MinRouteSecurity),
		)
	}

	return buyRegions, sellRegions
}

func (s *Server) stationCacheMetaForFlipScan(
	params engine.ScanParams,
	multiRegion bool,
	includeSourceBuy bool,
) stationCacheMeta {
	buyRegions, sellRegions := s.flipScanRegionScopes(params, multiRegion)
	regionUnion := mergeRegionSets(buyRegions, sellRegions)

	if s == nil || s.esi == nil {
		return stationCacheMeta{Regions: len(regionUnion)}
	}

	// Scanner dependencies:
	// - sell orders are used from BOTH source (buy) and destination (sell) region sets
	//   (destination sell-book is used for S2B/BfS and sell-order mode metrics).
	// - buy orders are used from destination (sell) region set.
	// - optionally, source-side buy orders are used when private-structure discovery is enabled.
	sellTypeRegions := mergeRegionSets(buyRegions, sellRegions)
	buyTypeRegions := mergeRegionSets(sellRegions)
	if includeSourceBuy {
		buyTypeRegions = mergeRegionSets(buyTypeRegions, buyRegions)
	}

	sellWindow := s.esi.OrderCacheWindow(mapRegionIDSet(sellTypeRegions), "sell")
	buyWindow := s.esi.OrderCacheWindow(mapRegionIDSet(buyTypeRegions), "buy")

	return stationCacheMetaFromWindows(len(regionUnion), sellWindow, buyWindow)
}

func (s *Server) regionScopeForContractScan(params engine.ScanParams) map[int32]bool {
	// Contracts are sourced from buy-side radius but liquidation can depend on sell-side market context.
	out := make(map[int32]bool)
	mergeRegionSet(out, s.regionsWithinRadius(params.CurrentSystemID, params.BuyRadius, params.MinRouteSecurity))
	mergeRegionSet(out, s.regionsWithinRadius(params.CurrentSystemID, params.SellRadius, params.MinRouteSecurity))
	return out
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	userCfg := s.loadConfigForUser(userID)

	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	params, err := s.parseScanParams(req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if req.IncludeStructures && s.sessions != nil {
		if token, tokenErr := s.sessions.EnsureValidTokenForUser(s.sso, userID); tokenErr == nil {
			params.AccessToken = token
		}
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()

	log.Printf("[API] Scan starting: system=%d, cargo=%.0f, buyR=%d, sellR=%d, margin=%.1f, tax=%.1f",
		params.CurrentSystemID, params.CargoCapacity, params.BuyRadius, params.SellRadius, params.MinMargin, params.SalesTaxPercent)

	startTime := time.Now()

	results, err := scanner.Scan(params, func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	})
	if err != nil {
		log.Printf("[API] Scan error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] Scan complete: %d results in %dms", len(results), durationMs)

	// Resolve structure names if user enabled the toggle
	if req.IncludeStructures {
		results = s.enrichStructureNames(userID, results)
	} else {
		results = filterFlipResultsExcludeStructures(results)
	}
	results = filterFlipResultsMarketDisabled(results)
	cacheMeta := s.stationCacheMetaForFlipScan(
		params,
		false,
		req.IncludeStructures && strings.TrimSpace(params.AccessToken) != "",
	)

	topProfit := 0.0
	totalProfit := 0.0
	for _, r := range results {
		kpiProfit := flipResultKPIProfit(r)
		if kpiProfit > topProfit {
			topProfit = kpiProfit
		}
		totalProfit += kpiProfit
	}
	scanID := s.db.InsertHistoryFull("radius", req.SystemName, len(results), topProfit, totalProfit, durationMs, req)
	go s.db.InsertFlipResults(scanID, results)
	var scanIDPtr *int64
	if scanID > 0 {
		scanIDPtr = &scanID
	}
	go s.processWatchlistAlerts(userID, userCfg, results, scanIDPtr)

	line, marshalErr := json.Marshal(map[string]interface{}{
		"type":       "result",
		"data":       results,
		"count":      len(results),
		"scan_id":    scanID,
		"cache_meta": cacheMeta,
	})
	if marshalErr != nil {
		log.Printf("[API] Scan JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleScanMultiRegion(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	userCfg := s.loadConfigForUser(userID)

	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	params, err := s.parseScanParams(req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if req.IncludeStructures && s.sessions != nil {
		if token, tokenErr := s.sessions.EnsureValidTokenForUser(s.sso, userID); tokenErr == nil {
			params.AccessToken = token
		}
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()

	log.Printf("[API] ScanMultiRegion starting: system=%d, cargo=%.0f, buyR=%d, sellR=%d, include_structures=%t, has_token=%t",
		params.CurrentSystemID,
		params.CargoCapacity,
		params.BuyRadius,
		params.SellRadius,
		req.IncludeStructures,
		strings.TrimSpace(params.AccessToken) != "",
	)

	startTime := time.Now()

	results, err := scanner.ScanMultiRegion(params, func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	})
	if err != nil {
		log.Printf("[API] ScanMultiRegion error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] ScanMultiRegion complete: %d results in %dms", len(results), durationMs)

	// Resolve structure names if user enabled the toggle
	if req.IncludeStructures {
		results = s.enrichStructureNames(userID, results)
	} else {
		results = filterFlipResultsExcludeStructures(results)
	}
	results = filterFlipResultsMarketDisabled(results)
	cacheMeta := s.stationCacheMetaForFlipScan(
		params,
		true,
		req.IncludeStructures && strings.TrimSpace(params.AccessToken) != "",
	)

	topProfit := 0.0
	totalProfit := 0.0
	for _, r := range results {
		kpiProfit := flipResultKPIProfit(r)
		if kpiProfit > topProfit {
			topProfit = kpiProfit
		}
		totalProfit += kpiProfit
	}
	scanID := s.db.InsertHistoryFull("region", req.SystemName, len(results), topProfit, totalProfit, durationMs, req)
	go s.db.InsertFlipResults(scanID, results)
	var scanIDPtr *int64
	if scanID > 0 {
		scanIDPtr = &scanID
	}
	go s.processWatchlistAlerts(userID, userCfg, results, scanIDPtr)

	line, marshalErr := json.Marshal(map[string]interface{}{
		"type":       "result",
		"data":       results,
		"count":      len(results),
		"scan_id":    scanID,
		"cache_meta": cacheMeta,
	})
	if marshalErr != nil {
		log.Printf("[API] ScanMultiRegion JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleScanRegionalDay(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	userCfg := s.loadConfigForUser(userID)

	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	params, err := s.parseScanParams(req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if req.IncludeStructures && s.sessions != nil {
		if token, tokenErr := s.sessions.EnsureValidTokenForUser(s.sso, userID); tokenErr == nil {
			params.AccessToken = token
		}
	}
	if params.TargetMarketSystemID <= 0 {
		writeError(w, 400, "target_market_system is required for regional day trader scan")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	sendProgress := func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()

	log.Printf("[API] ScanRegionalDay starting: system=%d, cargo=%.0f, buyR=%d, targetRegion=%d, period=%d, include_structures=%t, has_token=%t",
		params.CurrentSystemID,
		params.CargoCapacity,
		params.BuyRadius,
		params.TargetRegionID,
		params.AvgPricePeriod,
		req.IncludeStructures,
		strings.TrimSpace(params.AccessToken) != "",
	)

	startTime := time.Now()

	results, err := scanner.ScanMultiRegion(params, sendProgress)
	if err != nil {
		log.Printf("[API] ScanRegionalDay error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	// Resolve structure names if user enabled the toggle
	if req.IncludeStructures {
		results = s.enrichStructureNames(userID, results)
	} else {
		results = filterFlipResultsExcludeStructures(results)
	}
	results = filterFlipResultsMarketDisabled(results)

	inventory := s.loadRegionalInventorySnapshot(
		userID,
		params.TargetRegionID,
		params.TargetMarketSystemID,
		params.TargetMarketLocationID,
		sendProgress,
	)
	hubs, totalItems, targetRegionName, periodDays := scanner.BuildRegionalDayTrader(params, results, inventory, sendProgress)
	dayRows := engine.FlattenRegionalDayHubs(hubs)

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] ScanRegionalDay complete: hubs=%d items=%d rows=%d raw=%d in %dms",
		len(hubs), totalItems, len(dayRows), len(results), durationMs)

	cacheMeta := s.stationCacheMetaForFlipScan(
		params,
		true,
		req.IncludeStructures && strings.TrimSpace(params.AccessToken) != "",
	)

	topProfit := 0.0
	totalProfit := 0.0
	for _, row := range dayRows {
		kpiProfit := row.DayPeriodProfit
		if kpiProfit == 0 {
			kpiProfit = row.RealProfit
		}
		if kpiProfit == 0 {
			kpiProfit = row.TotalProfit
		}
		if kpiProfit > topProfit {
			topProfit = kpiProfit
		}
		totalProfit += kpiProfit
	}
	if len(dayRows) == 0 {
		for _, r := range results {
			kpiProfit := flipResultKPIProfit(r)
			if kpiProfit > topProfit {
				topProfit = kpiProfit
			}
			totalProfit += kpiProfit
		}
	}
	historyCount := len(dayRows)
	if historyCount == 0 {
		historyCount = len(results)
	}
	scanID := s.db.InsertHistoryFull("region", req.SystemName, historyCount, topProfit, totalProfit, durationMs, req)
	if scanID > 0 && len(dayRows) > 0 {
		go s.db.InsertRegionalDayResults(scanID, dayRows)
	}
	var scanIDPtr *int64
	if scanID > 0 {
		scanIDPtr = &scanID
	}
	alertRows := results
	if len(dayRows) > 0 {
		alertRows = dayRows
	}
	go s.processWatchlistAlerts(userID, userCfg, alertRows, scanIDPtr)

	line, marshalErr := json.Marshal(map[string]interface{}{
		"type":               "result",
		"data":               dayRows,
		"count":              len(dayRows),
		"scan_id":            scanID,
		"cache_meta":         cacheMeta,
		"target_region_name": targetRegionName,
		"period_days":        periodDays,
	})
	if marshalErr != nil {
		log.Printf("[API] ScanRegionalDay JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) loadRegionalInventorySnapshot(
	userID string,
	targetRegionID int32,
	targetMarketSystemID int32,
	targetMarketLocationID int64,
	progress func(string),
) *engine.RegionalInventorySnapshot {
	if s.sessions == nil || s.esi == nil || s.sso == nil {
		return nil
	}
	sessions := s.sessions.ListForUser(userID)
	if len(sessions) == 0 {
		return nil
	}
	if progress != nil {
		progress("Loading inventory and active orders...")
	}

	snapshot := &engine.RegionalInventorySnapshot{
		AssetsByType:     make(map[int32]int64),
		ActiveSellByType: make(map[int32]int64),
	}
	charactersUsed := 0

	for _, sess := range sessions {
		token, err := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if err != nil {
			continue
		}

		gotAny := false

		orders, orderErr := s.esi.GetCharacterOrders(sess.CharacterID, token)
		if orderErr == nil {
			for _, o := range orders {
				if o.TypeID <= 0 || o.IsBuyOrder || o.VolumeRemain <= 0 {
					continue
				}
				if targetMarketLocationID > 0 && o.LocationID != targetMarketLocationID {
					continue
				}
				if targetRegionID > 0 && o.RegionID != targetRegionID {
					continue
				}
				if targetMarketSystemID > 0 && targetMarketLocationID == 0 {
					if !s.matchesSystemByLocationID(o.LocationID, targetMarketSystemID) {
						continue
					}
				}
				snapshot.ActiveSellByType[o.TypeID] += int64(o.VolumeRemain)
			}
			gotAny = true
		}

		assets, assetsErr := s.esi.GetCharacterAssets(sess.CharacterID, token)
		if assetsErr == nil {
			assetByItemID := make(map[int64]esi.CharacterAsset, len(assets))
			for _, a := range assets {
				if a.ItemID > 0 {
					assetByItemID[a.ItemID] = a
				}
			}
			for _, a := range assets {
				if a.TypeID <= 0 || a.IsBlueprintCopy {
					continue
				}
				qty := a.Quantity
				if qty <= 0 {
					if a.IsSingleton {
						qty = 1
					} else {
						continue
					}
				}
				rootLocationID := resolveAssetRootLocationID(a.LocationID, assetByItemID)
				if !s.assetLocationMatchesTargetScope(
					rootLocationID,
					targetRegionID,
					targetMarketSystemID,
					targetMarketLocationID,
				) {
					continue
				}
				snapshot.AssetsByType[a.TypeID] += qty
			}
			gotAny = true
		}

		if gotAny {
			charactersUsed++
		}
	}

	if len(snapshot.AssetsByType) == 0 && len(snapshot.ActiveSellByType) == 0 {
		return nil
	}
	if progress != nil {
		progress(fmt.Sprintf(
			"Inventory synced: %d characters, %d asset types, %d active sell types",
			charactersUsed,
			len(snapshot.AssetsByType),
			len(snapshot.ActiveSellByType),
		))
	}
	return snapshot
}

func resolveAssetRootLocationID(locationID int64, byItemID map[int64]esi.CharacterAsset) int64 {
	if locationID <= 0 || len(byItemID) == 0 {
		return locationID
	}
	current := locationID
	seen := make(map[int64]bool, 8)
	for current > 0 && !seen[current] {
		seen[current] = true
		parent, ok := byItemID[current]
		if !ok {
			break
		}
		if parent.LocationID > 0 {
			current = parent.LocationID
		} else {
			break
		}
	}
	return current
}

func (s *Server) assetLocationMatchesTargetScope(
	locationID int64,
	targetRegionID int32,
	targetMarketSystemID int32,
	targetMarketLocationID int64,
) bool {
	if targetMarketLocationID > 0 {
		return locationID == targetMarketLocationID
	}
	if targetMarketSystemID > 0 {
		return s.matchesSystemByLocationID(locationID, targetMarketSystemID)
	}
	if targetRegionID > 0 {
		return s.matchesRegionByLocationID(locationID, targetRegionID)
	}
	return true
}

func (s *Server) matchesSystemByLocationID(locationID int64, systemID int32) bool {
	if locationID <= 0 || systemID <= 0 {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.sdeData == nil {
		return false
	}
	if st, ok := s.sdeData.Stations[locationID]; ok {
		return st.SystemID == systemID
	}
	if s.esi != nil {
		if sid, ok := s.esi.StructureSystemID(locationID); ok {
			return sid == systemID
		}
	}
	return false
}

func (s *Server) matchesRegionByLocationID(locationID int64, regionID int32) bool {
	if locationID <= 0 || regionID <= 0 {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.sdeData == nil {
		return false
	}
	if st, ok := s.sdeData.Stations[locationID]; ok {
		if sys, okSys := s.sdeData.Systems[st.SystemID]; okSys {
			return sys.RegionID == regionID
		}
		return false
	}
	if s.esi != nil {
		if sid, ok := s.esi.StructureSystemID(locationID); ok {
			if sys, okSys := s.sdeData.Systems[sid]; okSys {
				return sys.RegionID == regionID
			}
		}
	}
	return false
}

func (s *Server) handleScanContracts(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	params, err := s.parseScanParams(req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()

	log.Printf("[API] ScanContracts starting: system=%d, buyR=%d, margin=%.1f, tax=%.1f",
		params.CurrentSystemID, params.BuyRadius, params.MinMargin, params.SalesTaxPercent)

	ctx := r.Context()
	startTime := time.Now()

	results, err := scanner.ScanContractsWithContext(ctx, params, func(msg string) {
		if ctx.Err() != nil {
			return
		}
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		if _, writeErr := fmt.Fprintf(w, "%s\n", line); writeErr != nil {
			return
		}
		flusher.Flush()
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[API] ScanContracts canceled: %v", err)
			return
		}
		log.Printf("[API] ScanContracts error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}
	if ctx.Err() != nil {
		return
	}

	durationMs := time.Since(startTime).Milliseconds()
	results = s.filterContractResultsMarketDisabled(results)
	log.Printf("[API] ScanContracts complete: %d results in %dms", len(results), durationMs)
	regionIDs := s.regionScopeForContractScan(params)
	cacheMeta := s.stationCacheMetaForRegions(regionIDs)

	topProfit := 0.0
	totalProfit := 0.0
	for _, r := range results {
		kpiProfit := contractResultKPIProfit(r)
		if kpiProfit > topProfit {
			topProfit = kpiProfit
		}
		totalProfit += kpiProfit
	}
	scanID := s.db.InsertHistoryFull("contracts", req.SystemName, len(results), topProfit, totalProfit, durationMs, req)
	if ctx.Err() == nil {
		go s.db.InsertContractResults(scanID, results)
	}

	line, marshalErr := json.Marshal(map[string]interface{}{
		"type":       "result",
		"data":       results,
		"count":      len(results),
		"scan_id":    scanID,
		"cache_meta": cacheMeta,
	})
	if marshalErr != nil {
		log.Printf("[API] ScanContracts JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	if ctx.Err() != nil {
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleRouteFind(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	var req struct {
		SystemName           string  `json:"system_name"`
		IgnoredSystemIDs     []int32 `json:"ignored_system_ids"`
		TargetSystemName     string  `json:"target_system_name"`
		CargoCapacity        float64 `json:"cargo_capacity"`
		MinMargin            float64 `json:"min_margin"`
		MinISKPerJump        float64 `json:"min_isk_per_jump"`
		SalesTaxPercent      float64 `json:"sales_tax_percent"`
		BrokerFeePercent     float64 `json:"broker_fee_percent"`
		SplitTradeFees       bool    `json:"split_trade_fees"`
		BuyBrokerFeePercent  float64 `json:"buy_broker_fee_percent"`
		SellBrokerFeePercent float64 `json:"sell_broker_fee_percent"`
		BuySalesTaxPercent   float64 `json:"buy_sales_tax_percent"`
		SellSalesTaxPercent  float64 `json:"sell_sales_tax_percent"`
		MinHops              int     `json:"min_hops"`
		MaxHops              int     `json:"max_hops"`
		MinRouteSecurity     float64 `json:"min_route_security"` // 0 = all; 0.45 = highsec only; 0.7 = min 0.7
		AllowEmptyHops       bool    `json:"allow_empty_hops"`
		IncludeStructures    bool    `json:"include_structures"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}
	req.SystemName = strings.TrimSpace(req.SystemName)
	req.TargetSystemName = strings.TrimSpace(req.TargetSystemName)
	if req.MinISKPerJump < 0 {
		req.MinISKPerJump = 0
	}
	if req.MinHops < 1 {
		req.MinHops = 2
	}
	if req.MaxHops < req.MinHops {
		req.MaxHops = req.MinHops + 2
	}
	if req.MaxHops > 25 {
		req.MaxHops = 25
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	var systems map[int32]*sde.SolarSystem
	if s.sdeData != nil {
		systems = s.sdeData.Systems
	}
	ignoredSystemIDs := normalizeIgnoredSystemIDs(systems, req.IgnoredSystemIDs)
	s.mu.RUnlock()

	params := engine.RouteParams{
		SystemName:           req.SystemName,
		IgnoredSystemIDs:     ignoredSystemIDs,
		TargetSystemName:     req.TargetSystemName,
		CargoCapacity:        req.CargoCapacity,
		MinMargin:            req.MinMargin,
		MinISKPerJump:        req.MinISKPerJump,
		SalesTaxPercent:      req.SalesTaxPercent,
		BrokerFeePercent:     req.BrokerFeePercent,
		SplitTradeFees:       req.SplitTradeFees,
		BuyBrokerFeePercent:  req.BuyBrokerFeePercent,
		SellBrokerFeePercent: req.SellBrokerFeePercent,
		BuySalesTaxPercent:   req.BuySalesTaxPercent,
		SellSalesTaxPercent:  req.SellSalesTaxPercent,
		MinHops:              req.MinHops,
		MaxHops:              req.MaxHops,
		MinRouteSecurity:     req.MinRouteSecurity,
		AllowEmptyHops:       req.AllowEmptyHops,
		IncludeStructures:    req.IncludeStructures,
	}

	log.Printf(
		"[API] RouteFind: system=%s target=%s cargo=%.0f margin=%.1f minISK/jump=%.1f empty=%t hops=%d-%d",
		req.SystemName,
		req.TargetSystemName,
		req.CargoCapacity,
		req.MinMargin,
		req.MinISKPerJump,
		req.AllowEmptyHops,
		req.MinHops,
		req.MaxHops,
	)

	startTime := time.Now()
	results, err := scanner.FindRoutes(params, func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	})
	if err != nil {
		log.Printf("[API] RouteFind error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] RouteFind complete: %d routes in %dms", len(results), durationMs)

	rawCount := len(results)

	// Resolve structure names if user enabled the toggle
	if req.IncludeStructures {
		results = s.enrichRouteStructureNames(userID, results)
	} else {
		results = filterRouteResultsExcludeStructures(results)
	}
	results = filterRouteResultsMarketDisabled(results)
	if len(results) != rawCount {
		log.Printf("[API] RouteFind post-filter: raw=%d final=%d (include_structures=%t)", rawCount, len(results), req.IncludeStructures)
		line, _ := json.Marshal(map[string]string{
			"type":    "progress",
			"message": fmt.Sprintf("Filtered routes: %d/%d remain", len(results), rawCount),
		})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	}

	var topProfit, totalProfit float64
	for _, r := range results {
		if r.TotalProfit > topProfit {
			topProfit = r.TotalProfit
		}
		totalProfit += r.TotalProfit
	}

	scanID := s.db.InsertHistoryFull("route", req.SystemName, len(results), topProfit, totalProfit, durationMs, req)
	go s.db.InsertRouteResults(scanID, results)

	line, marshalErr := json.Marshal(map[string]interface{}{"type": "result", "data": results, "count": len(results), "scan_id": scanID})
	if marshalErr != nil {
		log.Printf("[API] RouteFind JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

// --- Watchlist ---

func (s *Server) handleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	items := s.db.GetWatchlistForUser(userID)
	filtered := make([]config.WatchlistItem, 0, len(items))
	for _, it := range items {
		if engine.IsMarketDisabledTypeID(it.TypeID) {
			continue
		}
		filtered = append(filtered, it)
	}
	writeJSON(w, filtered)
}

func (s *Server) handleAddWatchlist(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	var item config.WatchlistItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	// Validate type_id against SDE
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()
	if sdeData != nil {
		if _, ok := sdeData.Types[item.TypeID]; !ok {
			writeError(w, 400, fmt.Sprintf("unknown type_id %d", item.TypeID))
			return
		}
		// Use canonical SDE name if client didn't provide one
		if item.TypeName == "" {
			item.TypeName = sdeData.Types[item.TypeID].Name
		}
	}

	if item.AlertMetric == "" {
		item.AlertMetric = "margin_percent"
	}
	if item.AlertThreshold <= 0 && item.AlertMinMargin > 0 {
		item.AlertThreshold = item.AlertMinMargin
	}
	if engine.IsMarketDisabledTypeID(item.TypeID) {
		writeError(w, 400, "type_id is market-disabled")
		return
	}
	if item.AlertThreshold > 0 && !item.AlertEnabled {
		item.AlertEnabled = true
	}

	item.AddedAt = time.Now().Format(time.RFC3339)
	inserted := s.db.AddWatchlistItemForUser(userID, item)

	type addResponse struct {
		Items    []config.WatchlistItem `json:"items"`
		Inserted bool                   `json:"inserted"`
	}
	items := s.db.GetWatchlistForUser(userID)
	filtered := make([]config.WatchlistItem, 0, len(items))
	for _, it := range items {
		if engine.IsMarketDisabledTypeID(it.TypeID) {
			continue
		}
		filtered = append(filtered, it)
	}
	writeJSON(w, addResponse{
		Items:    filtered,
		Inserted: inserted,
	})
}

func (s *Server) handleDeleteWatchlist(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	idStr := r.PathValue("typeID")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, 400, "invalid type_id")
		return
	}
	s.db.DeleteWatchlistItemForUser(userID, int32(id))
	items := s.db.GetWatchlistForUser(userID)
	filtered := make([]config.WatchlistItem, 0, len(items))
	for _, it := range items {
		if engine.IsMarketDisabledTypeID(it.TypeID) {
			continue
		}
		filtered = append(filtered, it)
	}
	writeJSON(w, filtered)
}

func (s *Server) handleUpdateWatchlist(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	idStr := r.PathValue("typeID")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, 400, "invalid type_id")
		return
	}
	var body struct {
		AlertMinMargin float64 `json:"alert_min_margin"`
		AlertEnabled   bool    `json:"alert_enabled"`
		AlertMetric    string  `json:"alert_metric"`
		AlertThreshold float64 `json:"alert_threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	switch body.AlertMetric {
	case "", "margin_percent", "total_profit", "profit_per_unit", "daily_volume":
		// ok
	default:
		writeError(w, 400, "invalid alert_metric")
		return
	}
	if body.AlertThreshold < 0 {
		writeError(w, 400, "alert_threshold must be >= 0")
		return
	}

	alertMetric := body.AlertMetric
	if alertMetric == "" {
		alertMetric = "margin_percent"
	}
	alertThreshold := body.AlertThreshold
	alertEnabled := body.AlertEnabled

	// Backward-compatible behavior for old clients sending only alert_min_margin.
	if alertThreshold <= 0 && body.AlertMinMargin > 0 {
		alertMetric = "margin_percent"
		alertThreshold = body.AlertMinMargin
		alertEnabled = true
	}

	s.db.UpdateWatchlistItemForUser(userID, int32(id), body.AlertMinMargin, alertEnabled, alertMetric, alertThreshold)
	items := s.db.GetWatchlistForUser(userID)
	filtered := make([]config.WatchlistItem, 0, len(items))
	for _, it := range items {
		if engine.IsMarketDisabledTypeID(it.TypeID) {
			continue
		}
		filtered = append(filtered, it)
	}
	writeJSON(w, filtered)
}

func (s *Server) handleGetAlertHistory(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	// Optional filter by type_id
	typeIDStr := r.URL.Query().Get("type_id")
	var typeID int32
	if typeIDStr != "" {
		id, err := strconv.Atoi(typeIDStr)
		if err != nil {
			writeError(w, 400, "invalid type_id")
			return
		}
		typeID = int32(id)
	}

	// Optional limit
	limitStr := r.URL.Query().Get("limit")
	limit := 100 // default
	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 0 {
			writeError(w, 400, "invalid limit")
			return
		}
		if l > 0 {
			limit = l
		}
	}

	// Optional offset
	offsetStr := r.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		o, err := strconv.Atoi(offsetStr)
		if err != nil || o < 0 {
			writeError(w, 400, "invalid offset")
			return
		}
		offset = o
	}

	history, err := s.db.GetAlertHistoryPageForUser(userID, typeID, limit, offset)
	if err != nil {
		log.Printf("[API] Failed to get alert history: %v", err)
		writeError(w, 500, "failed to retrieve alert history")
		return
	}

	writeJSON(w, history)
}

// --- Station Trading ---

func (s *Server) handleScanStation(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	userCfg := s.loadConfigForUser(userID)

	var req struct {
		StationID            int64   `json:"station_id"`  // 0 = all stations
		RegionID             int32   `json:"region_id"`   // required
		SystemName           string  `json:"system_name"` // for radius-based scan
		IgnoredSystemIDs     []int32 `json:"ignored_system_ids"`
		Radius               int     `json:"radius"` // 0 = single system
		MinMargin            float64 `json:"min_margin"`
		SalesTaxPercent      float64 `json:"sales_tax_percent"`
		BrokerFee            float64 `json:"broker_fee"`
		CTSProfile           string  `json:"cts_profile"`
		SplitTradeFees       bool    `json:"split_trade_fees"`
		BuyBrokerFeePercent  float64 `json:"buy_broker_fee_percent"`
		SellBrokerFeePercent float64 `json:"sell_broker_fee_percent"`
		BuySalesTaxPercent   float64 `json:"buy_sales_tax_percent"`
		SellSalesTaxPercent  float64 `json:"sell_sales_tax_percent"`
		MinDailyVolume       int64   `json:"min_daily_volume"`
		// EVE Guru Profit Filters
		MinItemProfit   float64 `json:"min_item_profit"`
		MinDemandPerDay float64 `json:"min_demand_per_day"` // legacy alias for min_s2b_per_day
		MinS2BPerDay    float64 `json:"min_s2b_per_day"`
		MinBfSPerDay    float64 `json:"min_bfs_per_day"`
		// Risk Profile
		AvgPricePeriod     int     `json:"avg_price_period"`
		MinPeriodROI       float64 `json:"min_period_roi"`
		BvSRatioMin        float64 `json:"bvs_ratio_min"`
		BvSRatioMax        float64 `json:"bvs_ratio_max"`
		MaxPVI             float64 `json:"max_pvi"`
		MaxSDS             int     `json:"max_sds"`
		LimitBuyToPriceLow bool    `json:"limit_buy_to_price_low"`
		FlagExtremePrices  bool    `json:"flag_extreme_prices"`
		// Player structures
		IncludeStructures bool    `json:"include_structures"`
		StructureIDs      []int64 `json:"structure_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	streamAlive := true
	progressFn := func(msg string) {
		if !streamAlive || ctx.Err() != nil {
			streamAlive = false
			return
		}
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
			streamAlive = false
			cancel()
			return
		}
		flusher.Flush()
	}

	// Build StationIDs and RegionIDs based on request params
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	stationIDs := make(map[int64]bool)
	regionIDs := make(map[int32]bool)
	allowedSystemsByRegion := make(map[int32]map[int32]bool)
	ignoredSystems := ignoredSystemSet(sdeData.Systems, req.IgnoredSystemIDs)
	historyLabel := ""
	radiusMode := req.Radius > 0 && req.SystemName != ""
	singleStationMode := !radiusMode && req.StationID > 0
	allStationsMode := !radiusMode && !singleStationMode

	if radiusMode {
		// Radius-based scan: find all systems within radius, collect their stations
		systemID, ok := sdeData.SystemByName[strings.ToLower(req.SystemName)]
		if !ok {
			writeError(w, 400, "unknown system")
			return
		}
		systems := sdeData.Universe.SystemsWithinRadius(systemID, req.Radius)
		for ignoredID := range ignoredSystems {
			delete(systems, ignoredID)
		}
		for _, st := range sdeData.Stations {
			if _, inRange := systems[st.SystemID]; inRange {
				stationIDs[st.ID] = true
			}
		}
		for sysID := range systems {
			if sys, ok2 := sdeData.Systems[sysID]; ok2 {
				regionIDs[sys.RegionID] = true
				sysSet, exists := allowedSystemsByRegion[sys.RegionID]
				if !exists {
					sysSet = make(map[int32]bool)
					allowedSystemsByRegion[sys.RegionID] = sysSet
				}
				sysSet[sysID] = true
			}
		}
		historyLabel = fmt.Sprintf("%s +%d jumps", req.SystemName, req.Radius)
	} else if singleStationMode {
		// Single station (NPC or structure)
		if st, ok := sdeData.Stations[req.StationID]; !(ok && ignoredSystems[st.SystemID]) {
			stationIDs[req.StationID] = true
		}
		regionIDs[req.RegionID] = true
		historyLabel = fmt.Sprintf("Station %d", req.StationID)
	} else {
		// All stations in region
		regionIDs[req.RegionID] = true
		historyLabel = fmt.Sprintf("Region %d (all)", req.RegionID)
	}

	// Merge explicit player structure IDs when scan mode is station-scoped.
	if req.IncludeStructures && len(req.StructureIDs) > 0 && !allStationsMode {
		for _, sid := range req.StructureIDs {
			stationIDs[sid] = true
		}
	}

	log.Printf("[API] ScanStation starting: stations=%d, regions=%d, margin=%.1f, tax=%.1f, broker=%.1f, cts_profile=%s",
		len(stationIDs), len(regionIDs), req.MinMargin, req.SalesTaxPercent, req.BrokerFee, strings.TrimSpace(req.CTSProfile))

	// Get auth token if available (for structure name resolution)
	accessToken := ""
	if req.IncludeStructures && s.sessions != nil {
		if token, err := s.sessions.EnsureValidTokenForUser(s.sso, userID); err == nil {
			accessToken = token
		}
	}

	startTime := time.Now()

	// Scan each region and merge results
	var allResults []engine.StationTrade
	for regionID := range regionIDs {
		if ctx.Err() != nil || !streamAlive {
			return
		}
		params := engine.StationTradeParams{
			StationIDs:           stationIDs,
			AllowedSystems:       allowedSystemsByRegion[regionID],
			IgnoredSystems:       ignoredSystems,
			RegionID:             regionID,
			MinMargin:            req.MinMargin,
			SalesTaxPercent:      req.SalesTaxPercent,
			BrokerFee:            req.BrokerFee,
			CTSProfile:           req.CTSProfile,
			SplitTradeFees:       req.SplitTradeFees,
			BuyBrokerFeePercent:  req.BuyBrokerFeePercent,
			SellBrokerFeePercent: req.SellBrokerFeePercent,
			BuySalesTaxPercent:   req.BuySalesTaxPercent,
			SellSalesTaxPercent:  req.SellSalesTaxPercent,
			MinDailyVolume:       req.MinDailyVolume,
			MinItemProfit:        req.MinItemProfit,
			MinDemandPerDay:      req.MinDemandPerDay,
			MinS2BPerDay:         req.MinS2BPerDay,
			MinBfSPerDay:         req.MinBfSPerDay,
			AvgPricePeriod:       req.AvgPricePeriod,
			MinPeriodROI:         req.MinPeriodROI,
			BvSRatioMin:          req.BvSRatioMin,
			BvSRatioMax:          req.BvSRatioMax,
			MaxPVI:               req.MaxPVI,
			MaxSDS:               req.MaxSDS,
			LimitBuyToPriceLow:   req.LimitBuyToPriceLow,
			FlagExtremePrices:    req.FlagExtremePrices,
			AccessToken:          accessToken,
			IncludeStructures:    req.IncludeStructures,
			Ctx:                  ctx,
		}
		// In all-stations mode keep StationIDs nil so the engine evaluates full region scope.
		if allStationsMode {
			params.StationIDs = nil
		}

		results, err := scanner.ScanStationTrades(params, progressFn)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil || !streamAlive {
				return
			}
			log.Printf("[API] ScanStation error (region %d): %v", regionID, err)
			line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
			_, _ = fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
			return
		}
		if ctx.Err() != nil || !streamAlive {
			return
		}
		allResults = append(allResults, results...)
	}
	if ctx.Err() != nil || !streamAlive {
		return
	}

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] ScanStation complete: %d results in %dms", len(allResults), durationMs)
	cacheMeta := s.stationCacheMetaForRegions(regionIDs)

	// Filter out player structures if toggle is OFF
	// (structure names are already resolved inside ScanStationTrades)
	if !req.IncludeStructures {
		allResults = filterStationTradesExcludeStructures(allResults)
	}
	allResults = filterStationTradesMarketDisabled(allResults)

	// Calculate totals
	topProfit := 0.0
	totalProfit := 0.0
	for _, r := range allResults {
		p := stationTradeKPIProfit(r)
		if p > topProfit {
			topProfit = p
		}
		totalProfit += p
	}

	// Save to history with full params
	scanID := s.db.InsertHistoryFull("station", historyLabel, len(allResults), topProfit, totalProfit, durationMs, req)
	if scanID > 0 {
		go s.db.InsertStationResults(scanID, allResults)
	}
	var scanIDPtr *int64
	if scanID > 0 {
		scanIDPtr = &scanID
	}
	go s.processWatchlistAlerts(userID, userCfg, allResults, scanIDPtr)

	line, marshalErr := json.Marshal(map[string]interface{}{
		"type":       "result",
		"data":       allResults,
		"count":      len(allResults),
		"scan_id":    scanID,
		"cache_meta": cacheMeta,
	})
	if marshalErr != nil {
		log.Printf("[API] ScanStation JSON marshal error: %v", marshalErr)
		errLine, _ := json.Marshal(map[string]string{"type": "error", "message": "JSON: " + marshalErr.Error()})
		fmt.Fprintf(w, "%s\n", errLine)
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleGetStations(w http.ResponseWriter, r *http.Request) {
	type stationInfo struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		SystemID    int32  `json:"system_id"`
		RegionID    int32  `json:"region_id"`
		IsStructure bool   `json:"is_structure,omitempty"`
	}
	type stationsResponse struct {
		Stations []stationInfo `json:"stations"`
		RegionID int32         `json:"region_id"`
		SystemID int32         `json:"system_id"`
	}

	systemName := strings.TrimSpace(r.URL.Query().Get("system"))
	if systemName == "" || !s.isReady() {
		writeJSON(w, stationsResponse{Stations: []stationInfo{}})
		return
	}

	s.mu.RLock()
	systemID, ok := s.sdeData.SystemByName[strings.ToLower(systemName)]
	stations := s.sdeData.Stations
	s.mu.RUnlock()

	if !ok {
		writeJSON(w, stationsResponse{Stations: []stationInfo{}})
		return
	}
	if len(stations) == 0 {
		// If station map isn't available yet, avoid false "no NPC stations" hints in UI.
		writeJSON(w, stationsResponse{Stations: []stationInfo{}})
		return
	}

	regionID := int32(0)
	if sys, ok2 := s.sdeData.Systems[systemID]; ok2 {
		regionID = sys.RegionID
	}

	// Collect NPC station IDs for this system
	var stationIDs []int64
	for _, st := range stations {
		if st.SystemID == systemID {
			stationIDs = append(stationIDs, st.ID)
		}
	}

	// Prefetch station names from ESI (uses cache)
	idMap := make(map[int64]bool, len(stationIDs))
	for _, id := range stationIDs {
		idMap[id] = true
	}
	s.esi.PrefetchStationNames(idMap)

	result := make([]stationInfo, 0, len(stationIDs))
	for _, id := range stationIDs {
		result = append(result, stationInfo{
			ID:       id,
			Name:     s.esi.StationName(id),
			SystemID: systemID,
			RegionID: regionID,
		})
	}

	writeJSON(w, stationsResponse{
		Stations: result,
		RegionID: regionID,
		SystemID: systemID,
	})
}

func (s *Server) handleAuthStructures(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, false)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}
	sess := selectedSessions[0]

	token, err := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
	if err != nil {
		writeError(w, 401, err.Error())
		return
	}

	systemIDStr := r.URL.Query().Get("system_id")
	regionIDStr := r.URL.Query().Get("region_id")
	if systemIDStr == "" || regionIDStr == "" {
		writeJSON(w, []interface{}{})
		return
	}

	systemID64, err1 := strconv.ParseInt(systemIDStr, 10, 32)
	regionID64, err2 := strconv.ParseInt(regionIDStr, 10, 32)
	if err1 != nil || err2 != nil {
		writeJSON(w, []interface{}{})
		return
	}
	systemID := int32(systemID64)
	regionID := int32(regionID64)

	structures, err := s.esi.FetchSystemStructures(systemID, regionID, token)
	if err != nil {
		log.Printf("[API] FetchSystemStructures error: %v", err)
		writeJSON(w, []interface{}{})
		return
	}

	type stationInfo struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		SystemID    int32  `json:"system_id"`
		RegionID    int32  `json:"region_id"`
		IsStructure bool   `json:"is_structure,omitempty"`
	}

	result := make([]stationInfo, 0, len(structures))
	skipped := 0
	for _, st := range structures {
		// Skip structures with placeholder names (no access or not in EVERef)
		if st.Name == "" || strings.HasPrefix(st.Name, "Structure ") || strings.HasPrefix(st.Name, "Location ") {
			skipped++
			continue
		}
		result = append(result, stationInfo{
			ID:          st.ID,
			Name:        st.Name,
			SystemID:    st.SystemID,
			RegionID:    st.RegionID,
			IsStructure: true,
		})
	}
	if skipped > 0 {
		log.Printf("[API] Filtered out %d inaccessible structures from dropdown", skipped)
	}
	writeJSON(w, result)
}

func filterExecutionPlanOrders(orders []esi.MarketOrder, typeID int32, systemID int32, locationID int64) []esi.MarketOrder {
	filtered := make([]esi.MarketOrder, 0, len(orders))
	for _, o := range orders {
		if o.TypeID != typeID {
			continue
		}
		if locationID != 0 {
			if o.LocationID != locationID {
				continue
			}
		} else if systemID != 0 && o.SystemID != systemID {
			continue
		}
		filtered = append(filtered, o)
	}
	return filtered
}

func (s *Server) handleExecutionPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TypeID     int32 `json:"type_id"`
		RegionID   int32 `json:"region_id"`
		SystemID   int32 `json:"system_id"`
		LocationID int64 `json:"location_id"` // 0 = whole region
		Quantity   int32 `json:"quantity"`
		IsBuy      bool  `json:"is_buy"`
		ImpactDays int   `json:"impact_days"` // 0 = use engine default (e.g. 30); from station trading "Period (days)"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if req.RegionID == 0 || req.TypeID == 0 || req.Quantity <= 0 {
		writeError(w, 400, "region_id, type_id and positive quantity required")
		return
	}

	// For buy we need sell orders (we walk the ask side); for sell we need buy orders (bid side)
	orderType := "sell"
	if !req.IsBuy {
		orderType = "buy"
	}
	var orders []esi.MarketOrder
	var err error
	if req.LocationID != 0 && isPlayerStructure(req.LocationID) {
		userID := userIDFromRequest(r)
		if userID == "" || s.sessions == nil || s.sso == nil {
			writeError(w, 401, "player structure market data requires authenticated character access")
			return
		}
		characterID, allScope, err := parseAuthScope(r)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, false)
		if err != nil || len(selectedSessions) == 0 {
			writeError(w, 401, "player structure market data requires an authenticated character")
			return
		}
		token, err := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, selectedSessions[0].CharacterID)
		if err != nil {
			log.Printf("[API] execution/plan EnsureValidTokenForUserCharacter: %v", err)
			writeError(w, 401, "failed to refresh character token")
			return
		}
		orders, err = s.esi.FetchStructureOrders(req.LocationID, token)
		if err != nil {
			log.Printf("[API] execution/plan FetchStructureOrders(%d): %v", req.LocationID, err)
			writeError(w, 502, "failed to fetch structure market orders")
			return
		}
	} else {
		orders, err = s.esi.FetchRegionOrders(req.RegionID, orderType)
		if err != nil {
			log.Printf("[API] execution/plan FetchRegionOrders: %v", err)
			writeError(w, 502, "failed to fetch market orders")
			return
		}
	}

	filtered := filterExecutionPlanOrders(orders, req.TypeID, req.SystemID, req.LocationID)

	result := engine.ComputeExecutionPlan(filtered, req.Quantity, req.IsBuy)

	// When market history is available, add impact calibration (Amihud, σ, TWAP slices)
	if s.db != nil {
		history, ok := s.db.GetMarketHistory(req.RegionID, req.TypeID)
		if !ok {
			entries, err := s.esi.FetchMarketHistory(req.RegionID, req.TypeID)
			if err == nil && len(entries) > 0 {
				s.db.SetMarketHistory(req.RegionID, req.TypeID, entries)
				history = entries
			}
		}
		if len(history) >= 5 {
			impactDays := req.ImpactDays
			if impactDays <= 0 {
				impactDays = engine.DefaultImpactDays
			}
			if impactDays > 365 {
				impactDays = 365
			}
			params := engine.CalibrateImpact(history, impactDays)
			if params.Valid {
				// Use best price from execution plan as reference for ISK conversion
				refPrice := result.BestPrice
				est := engine.EstimateImpact(params, float64(req.Quantity), refPrice)
				result.Impact = &est
			}
		}
	}

	writeJSON(w, result)
}

// --- Scan History ---

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	writeJSON(w, s.db.GetHistory(limit))
}

func (s *Server) handleGetHistoryByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	record := s.db.GetHistoryByID(id)
	if record == nil {
		writeError(w, 404, "not found")
		return
	}
	writeJSON(w, record)
}

func (s *Server) regionalDayParamsFromHistory(record *db.ScanRecord) (engine.ScanParams, bool) {
	if record == nil || len(record.Params) == 0 {
		return engine.ScanParams{}, false
	}

	var req scanRequest
	if err := json.Unmarshal(record.Params, &req); err != nil {
		return engine.ScanParams{}, false
	}

	targetMarketSystem := strings.TrimSpace(req.TargetMarketSystem)
	// Region scans without destination market system are classic multi-region scans.
	// Do not reinterpret them as regional day-trader rows.
	if targetMarketSystem == "" {
		return engine.ScanParams{}, false
	}

	params := engine.ScanParams{
		IgnoredSystemIDs:       req.IgnoredSystemIDs,
		CargoCapacity:          req.CargoCapacity,
		BuyRadius:              req.BuyRadius,
		SellRadius:             req.SellRadius,
		MinMargin:              req.MinMargin,
		SalesTaxPercent:        req.SalesTaxPercent,
		BrokerFeePercent:       req.BrokerFeePercent,
		SplitTradeFees:         req.SplitTradeFees,
		BuyBrokerFeePercent:    req.BuyBrokerFeePercent,
		SellBrokerFeePercent:   req.SellBrokerFeePercent,
		BuySalesTaxPercent:     req.BuySalesTaxPercent,
		SellSalesTaxPercent:    req.SellSalesTaxPercent,
		MinDailyVolume:         req.MinDailyVolume,
		MaxInvestment:          req.MaxInvestment,
		MinItemProfit:          req.MinItemProfit,
		MinPeriodROI:           req.MinPeriodROI,
		MaxDOS:                 req.MaxDOS,
		MinDemandPerDay:        req.MinDemandPerDay,
		PurchaseDemandDays:     req.PurchaseDemandDays,
		MinS2BPerDay:           req.MinS2BPerDay,
		MinBfSPerDay:           req.MinBfSPerDay,
		MinS2BBfSRatio:         req.MinS2BBfSRatio,
		MaxS2BBfSRatio:         req.MaxS2BBfSRatio,
		AvgPricePeriod:         req.AvgPricePeriod,
		ShippingCostPerM3Jump:  req.ShippingCostPerM3Jump,
		MinRouteSecurity:       req.MinRouteSecurity,
		TargetMarketLocationID: req.TargetMarketLocationID,
		CategoryIDs:            req.CategoryIDs,
		SellOrderMode:          req.SellOrderMode,
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.sdeData == nil {
		return params, true
	}
	params.IgnoredSystemIDs = normalizeIgnoredSystemIDs(s.sdeData.Systems, req.IgnoredSystemIDs)

	if rid, ok := s.sdeData.RegionByName[strings.ToLower(strings.TrimSpace(req.TargetRegion))]; ok && rid > 0 {
		params.TargetRegionID = rid
	}
	if sid, ok := s.sdeData.SystemByName[strings.ToLower(targetMarketSystem)]; ok && sid > 0 {
		params.TargetMarketSystemID = sid
		if sys, okSys := s.sdeData.Systems[sid]; okSys && sys.RegionID > 0 {
			params.TargetRegionID = sys.RegionID
		}
	}

	return params, true
}

func (s *Server) rebuildRegionalHistoryRows(record *db.ScanRecord, raw []engine.FlipResult) []engine.FlipResult {
	if record == nil || len(raw) == 0 {
		return nil
	}

	params, ok := s.regionalDayParamsFromHistory(record)
	if !ok {
		return nil
	}

	s.mu.RLock()
	scanner := s.scanner
	s.mu.RUnlock()
	if scanner == nil {
		return nil
	}

	// History replay should be deterministic and local: use cached market history
	// only, without live ESI refetches.
	offline := *scanner
	offline.ESI = nil

	hubs, _, _, _ := (&offline).BuildRegionalDayTrader(params, raw, nil, nil)
	rows := engine.FlattenRegionalDayHubs(hubs)
	if len(rows) == 0 {
		return nil
	}
	return rows
}

func (s *Server) handleGetHistoryResults(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	record := s.db.GetHistoryByID(id)
	if record == nil {
		writeError(w, 404, "not found")
		return
	}

	var results interface{}
	switch record.Tab {
	case "station":
		results = filterStationTradesMarketDisabled(s.db.GetStationResults(id))
	case "region":
		regionRows := filterFlipResultsMarketDisabled(s.db.GetRegionalDayResults(id))
		if len(regionRows) > 0 {
			results = regionRows
		} else {
			rawRows := s.db.GetFlipResults(id)
			rebuilt := s.rebuildRegionalHistoryRows(record, rawRows)
			if len(rebuilt) > 0 {
				regionRows = filterFlipResultsMarketDisabled(rebuilt)
				if len(regionRows) > 0 {
					go s.db.InsertRegionalDayResults(id, regionRows)
					results = regionRows
					break
				}
			}
			// Backward compatibility for scans where a deterministic rebuild is not possible.
			results = filterFlipResultsMarketDisabled(rawRows)
		}
	case "contracts":
		contractResults := s.db.GetContractResults(id)
		results = s.filterContractResultsMarketDisabled(contractResults)
	case "route":
		results = filterRouteResultsMarketDisabled(s.db.GetRouteResults(id))
	default:
		results = filterFlipResultsMarketDisabled(s.db.GetFlipResults(id))
	}

	writeJSON(w, map[string]interface{}{
		"scan":    record,
		"results": results,
	})
}

func (s *Server) handleDeleteHistory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	if err := s.db.DeleteHistory(id); err != nil {
		writeError(w, 500, "delete failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleClearHistory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OlderThanDays int `json:"older_than_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.OlderThanDays = 7 // default: clear older than 7 days
	}
	if req.OlderThanDays < 1 {
		req.OlderThanDays = 7
	}
	count, err := s.db.ClearHistory(req.OlderThanDays)
	if err != nil {
		writeError(w, 500, "clear failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"status": "cleared", "deleted": count})
}

// --- Auth ---

type authCharacterSummary struct {
	CharacterID   int64  `json:"character_id"`
	CharacterName string `json:"character_name"`
	Active        bool   `json:"active"`
}

func parseAuthScope(r *http.Request) (characterID int64, all bool, err error) {
	scope := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("scope")))
	charParam := strings.TrimSpace(r.URL.Query().Get("character_id"))

	if scope == "all" || strings.EqualFold(charParam, "all") {
		if charParam != "" && !strings.EqualFold(charParam, "all") {
			return 0, false, fmt.Errorf("character_id and scope=all cannot be combined")
		}
		return 0, true, nil
	}

	if charParam == "" {
		return 0, false, nil
	}
	id, parseErr := strconv.ParseInt(charParam, 10, 64)
	if parseErr != nil || id <= 0 {
		return 0, false, fmt.Errorf("invalid character_id")
	}
	return id, false, nil
}

func (s *Server) authSessionsForScope(userID string, characterID int64, all bool, allowAll bool) ([]*auth.Session, error) {
	if s.sessions == nil {
		return nil, fmt.Errorf("not logged in")
	}
	if all {
		if !allowAll {
			return nil, fmt.Errorf("scope=all is not supported for this endpoint")
		}
		allSessions := s.sessions.ListForUser(userID)
		if len(allSessions) == 0 {
			return nil, fmt.Errorf("not logged in")
		}
		return allSessions, nil
	}
	if characterID > 0 {
		sess := s.sessions.GetByCharacterIDForUser(userID, characterID)
		if sess == nil {
			return nil, fmt.Errorf("character not logged in")
		}
		return []*auth.Session{sess}, nil
	}
	sess := s.sessions.GetForUser(userID)
	if sess == nil {
		return nil, fmt.Errorf("not logged in")
	}
	return []*auth.Session{sess}, nil
}

func (s *Server) requireIndustryAuthUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID := userIDFromRequest(r)
	if s.sessions == nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return "", false
	}
	if s.sessions.GetForUser(userID) == nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return "", false
	}
	return userID, true
}

func (s *Server) authStatusPayload(userID string) map[string]interface{} {
	revision := s.authRevisionForUser(userID)
	if s.sessions == nil {
		return map[string]interface{}{
			"logged_in":     false,
			"auth_revision": revision,
		}
	}
	active := s.sessions.GetForUser(userID)
	if active == nil {
		return map[string]interface{}{
			"logged_in":     false,
			"auth_revision": revision,
		}
	}
	all := s.sessions.ListForUser(userID)
	characters := make([]authCharacterSummary, 0, len(all))
	for _, sess := range all {
		characters = append(characters, authCharacterSummary{
			CharacterID:   sess.CharacterID,
			CharacterName: sess.CharacterName,
			Active:        sess.Active,
		})
	}
	return map[string]interface{}{
		"logged_in":      true,
		"character_id":   active.CharacterID,
		"character_name": active.CharacterName,
		"characters":     characters,
		"auth_revision":  revision,
	}
}

func (s *Server) writeAuthStatus(w http.ResponseWriter, userID string) {
	writeJSON(w, s.authStatusPayload(userID))
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.sso == nil {
		writeError(w, 500, "SSO not configured")
		return
	}
	state := auth.GenerateState()
	desktop := r.URL.Query().Get("desktop") == "1"
	userID := userIDFromRequest(r)

	s.ssoStatesMu.Lock()
	// Purge expired states
	now := time.Now()
	for k, v := range s.ssoStates {
		if now.After(v.ExpiresAt) {
			delete(s.ssoStates, k)
		}
	}
	s.ssoStates[state] = ssoStateEntry{
		ExpiresAt: now.Add(10 * time.Minute),
		Desktop:   desktop,
		UserID:    userID,
	}
	s.ssoStatesMu.Unlock()
	authURL := s.sso.BuildAuthURL(state)
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("mode")), "json") {
		writeJSON(w, map[string]string{"url": authURL})
		return
	}

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.sso == nil {
		writeError(w, 500, "SSO not configured")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	s.ssoStatesMu.Lock()
	entry, ok := s.ssoStates[state]
	if ok {
		delete(s.ssoStates, state) // consume: one-time use
	}
	s.ssoStatesMu.Unlock()

	if state == "" || !ok || time.Now().After(entry.ExpiresAt) {
		writeError(w, 400, "invalid or expired state parameter")
		return
	}

	// Exchange code for tokens
	tok, err := s.sso.ExchangeCode(code)
	if err != nil {
		log.Printf("[AUTH] Exchange error: %v", err)
		writeError(w, 500, "token exchange failed: "+err.Error())
		return
	}

	// Verify token to get character info
	info, err := auth.VerifyToken(tok.AccessToken)
	if err != nil {
		log.Printf("[AUTH] Verify error: %v", err)
		writeError(w, 500, "token verify failed: "+err.Error())
		return
	}

	// Save session
	userID := strings.TrimSpace(entry.UserID)
	if !isValidUserID(userID) {
		userID = userIDFromRequest(r)
	}
	userID = s.setUserIDCookie(w, r, userID)
	sess := &auth.Session{
		CharacterID:   info.CharacterID,
		CharacterName: info.CharacterName,
		AccessToken:   tok.AccessToken,
		RefreshToken:  tok.RefreshToken,
		ExpiresAt:     time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second),
	}
	if err := s.sessions.SaveAndActivateForUser(userID, sess); err != nil {
		log.Printf("[AUTH] Save session error: %v", err)
		writeError(w, 500, "save session failed")
		return
	}
	s.bumpAuthRevision(userID)

	log.Printf("[AUTH] Logged in as %s (ID: %d)", info.CharacterName, info.CharacterID)

	// Check whether the login was initiated from the desktop (Tauri) app.
	if !entry.Desktop {
		// Web browser: redirect back to the frontend (original behaviour).
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	// Desktop / Tauri: show a styled success page in the system browser.
	// The Tauri app detects login via polling /api/auth/status.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>EVE Flipper - Login</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0d1117;color:#c9d1d9;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif;
display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{text-align:center;padding:3rem 4rem;border:1px solid #30363d;border-radius:12px;background:#161b22}
.avatar{width:64px;height:64px;border-radius:8px;margin-bottom:1rem}
h1{font-size:1.5rem;color:#58a6ff;margin-bottom:.5rem}
p{color:#8b949e;margin-bottom:.25rem}
.hint{margin-top:1.5rem;font-size:.85rem;color:#484f58}
</style></head>
<body><div class="card">
<img class="avatar" src="https://images.evetech.net/characters/%d/portrait?size=128" alt="">
<h1>%s</h1>
<p>Login successful!</p>
<p class="hint">You can close this tab and return to EVE Flipper.</p>
</div>
<script>setTimeout(function(){window.close()},4000)</script>
</body></html>`, info.CharacterID, info.CharacterName)
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	s.writeAuthStatus(w, userID)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.sessions != nil {
		s.sessions.DeleteForUser(userID)
	}
	s.bumpAuthRevision(userID)
	s.clearWalletTxnCache()
	log.Println("[AUTH] Logged out")
	s.writeAuthStatus(w, userID)
}

func (s *Server) handleAuthCharacterSelect(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.sessions == nil {
		writeError(w, 401, "not logged in")
		return
	}
	var req struct {
		CharacterID int64 `json:"character_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if req.CharacterID <= 0 {
		writeError(w, 400, "character_id is required")
		return
	}
	if err := s.sessions.SetActiveForUser(userID, req.CharacterID); err != nil {
		writeError(w, 404, err.Error())
		return
	}
	s.bumpAuthRevision(userID)
	s.clearWalletTxnCache()
	s.writeAuthStatus(w, userID)
}

func (s *Server) handleAuthCharacterDelete(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.sessions == nil {
		writeError(w, 401, "not logged in")
		return
	}
	characterID, err := strconv.ParseInt(r.PathValue("characterID"), 10, 64)
	if err != nil || characterID <= 0 {
		writeError(w, 400, "invalid characterID")
		return
	}
	if err := s.sessions.DeleteByCharacterIDForUser(userID, characterID); err != nil {
		writeError(w, 500, "delete failed: "+err.Error())
		return
	}
	s.bumpAuthRevision(userID)
	s.clearWalletTxnCache()
	s.writeAuthStatus(w, userID)
}

func (s *Server) handleAuthCharacter(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	type charInfo struct {
		CharacterID   int64                        `json:"character_id"`
		CharacterName string                       `json:"character_name"`
		Wallet        float64                      `json:"wallet"`
		Orders        []esi.CharacterOrder         `json:"orders"`
		OrderHistory  []esi.HistoricalOrder        `json:"order_history"`
		Transactions  []esi.WalletTransaction      `json:"transactions"`
		Skills        *esi.SkillSheet              `json:"skills"`
		Risk          *engine.PortfolioRiskSummary `json:"risk,omitempty"`
	}

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, true)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}

	fetchOne := func(sess *auth.Session) (*charInfo, error) {
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			return nil, tokenErr
		}

		result := &charInfo{
			CharacterID:   sess.CharacterID,
			CharacterName: sess.CharacterName,
		}

		// Fetch all character data in parallel for faster popup loading.
		var wgChar sync.WaitGroup
		var muChar sync.Mutex

		wgChar.Add(5)

		go func() {
			defer wgChar.Done()
			if balance, fetchErr := s.esi.GetWalletBalance(sess.CharacterID, token); fetchErr == nil {
				muChar.Lock()
				result.Wallet = balance
				muChar.Unlock()
			} else {
				log.Printf("[AUTH] Wallet error (%s): %v", sess.CharacterName, fetchErr)
			}
		}()

		go func() {
			defer wgChar.Done()
			if orders, fetchErr := s.esi.GetCharacterOrders(sess.CharacterID, token); fetchErr == nil {
				muChar.Lock()
				result.Orders = orders
				muChar.Unlock()
			} else {
				log.Printf("[AUTH] Orders error (%s): %v", sess.CharacterName, fetchErr)
			}
		}()

		go func() {
			defer wgChar.Done()
			if history, fetchErr := s.esi.GetOrderHistory(sess.CharacterID, token); fetchErr == nil {
				muChar.Lock()
				result.OrderHistory = history
				muChar.Unlock()
			} else {
				log.Printf("[AUTH] Order history error (%s): %v", sess.CharacterName, fetchErr)
			}
		}()

		go func() {
			defer wgChar.Done()
			if txns, fetchErr := s.esi.GetWalletTransactions(sess.CharacterID, token); fetchErr == nil {
				muChar.Lock()
				result.Transactions = txns
				muChar.Unlock()
			} else {
				log.Printf("[AUTH] Transactions error (%s): %v", sess.CharacterName, fetchErr)
			}
		}()

		go func() {
			defer wgChar.Done()
			if skills, fetchErr := s.esi.GetSkills(sess.CharacterID, token); fetchErr == nil {
				muChar.Lock()
				result.Skills = skills
				muChar.Unlock()
			} else {
				log.Printf("[AUTH] Skills error (%s): %v", sess.CharacterName, fetchErr)
			}
		}()

		wgChar.Wait()
		return result, nil
	}

	collected := make([]*charInfo, 0, len(selectedSessions))
	for _, sess := range selectedSessions {
		info, fetchErr := fetchOne(sess)
		if fetchErr != nil {
			log.Printf("[AUTH] Failed to fetch character (%s): %v", sess.CharacterName, fetchErr)
			if !allScope {
				writeError(w, 401, fetchErr.Error())
				return
			}
			continue
		}
		collected = append(collected, info)
	}
	if len(collected) == 0 {
		writeError(w, 401, "failed to fetch character data")
		return
	}

	var result charInfo
	if allScope {
		result = charInfo{
			CharacterID:   0,
			CharacterName: "All Characters",
		}
		for _, part := range collected {
			result.Wallet += part.Wallet
			result.Orders = append(result.Orders, part.Orders...)
			result.OrderHistory = append(result.OrderHistory, part.OrderHistory...)
			result.Transactions = append(result.Transactions, part.Transactions...)
		}
	} else {
		result = *collected[0]
	}

	// Enrich orders with type/location names
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	if sdeData != nil {
		// Collect all location IDs for prefetch
		locationIDs := make(map[int64]bool)
		for _, o := range result.Orders {
			locationIDs[o.LocationID] = true
		}
		for _, o := range result.OrderHistory {
			locationIDs[o.LocationID] = true
		}
		for _, t := range result.Transactions {
			locationIDs[t.LocationID] = true
		}
		s.esi.PrefetchStationNames(locationIDs)

		// Enrich active orders
		for i := range result.Orders {
			if t, ok := sdeData.Types[result.Orders[i].TypeID]; ok {
				result.Orders[i].TypeName = t.Name
			}
			result.Orders[i].LocationName = s.esi.StationName(result.Orders[i].LocationID)
		}

		// Enrich order history
		for i := range result.OrderHistory {
			if t, ok := sdeData.Types[result.OrderHistory[i].TypeID]; ok {
				result.OrderHistory[i].TypeName = t.Name
			}
			result.OrderHistory[i].LocationName = s.esi.StationName(result.OrderHistory[i].LocationID)
		}

		// Enrich transactions
		for i := range result.Transactions {
			if t, ok := sdeData.Types[result.Transactions[i].TypeID]; ok {
				result.Transactions[i].TypeName = t.Name
			}
			result.Transactions[i].LocationName = s.esi.StationName(result.Transactions[i].LocationID)
		}
	}

	// Compute portfolio risk summary from recent wallet transactions.
	if len(result.Transactions) > 0 {
		if risk := engine.ComputePortfolioRiskFromTransactions(result.Transactions); risk != nil {
			result.Risk = risk
		}
	}

	writeJSON(w, result)
}

func (s *Server) handleAuthLocation(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, false)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}
	sess := selectedSessions[0]

	token, err := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
	if err != nil {
		writeError(w, 401, err.Error())
		return
	}

	loc, err := s.esi.GetCharacterLocation(sess.CharacterID, token)
	if err != nil {
		writeError(w, 500, "failed to get location: "+err.Error())
		return
	}

	// Resolve system name from SDE
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	result := struct {
		SolarSystemID   int32  `json:"solar_system_id"`
		SolarSystemName string `json:"solar_system_name"`
		StationID       int64  `json:"station_id,omitempty"`
		StationName     string `json:"station_name,omitempty"`
	}{
		SolarSystemID: loc.SolarSystemID,
	}

	if sdeData != nil {
		if sys, ok := sdeData.Systems[loc.SolarSystemID]; ok {
			result.SolarSystemName = sys.Name
		}
	}

	// Get station name if docked
	if loc.StationID != 0 {
		result.StationID = loc.StationID
		result.StationName = s.esi.StationName(loc.StationID)
	} else if loc.StructureID != 0 {
		result.StationID = loc.StructureID
		result.StationName = s.esi.StationName(loc.StructureID)
	}

	writeJSON(w, result)
}

func (s *Server) handleAuthUndercuts(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, true)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}

	// Fetch active orders for the selected scope.
	var orders []esi.CharacterOrder
	for _, sess := range selectedSessions {
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			log.Printf("[AUTH] Undercuts token error (%s): %v", sess.CharacterName, tokenErr)
			if !allScope {
				writeError(w, 401, tokenErr.Error())
				return
			}
			continue
		}
		charOrders, fetchErr := s.esi.GetCharacterOrders(sess.CharacterID, token)
		if fetchErr != nil {
			log.Printf("[AUTH] Undercuts orders error (%s): %v", sess.CharacterName, fetchErr)
			if !allScope {
				writeError(w, 500, "failed to fetch orders: "+fetchErr.Error())
				return
			}
			continue
		}
		orders = append(orders, charOrders...)
	}

	if len(orders) == 0 {
		writeJSON(w, []engine.UndercutStatus{})
		return
	}

	// Collect unique (region, type) pairs.
	type regionType struct {
		regionID int32
		typeID   int32
	}
	pairs := make(map[regionType]bool)
	for _, o := range orders {
		pairs[regionType{o.RegionID, o.TypeID}] = true
	}

	// Fetch regional orders for each unique type (concurrently, with semaphore).
	// Limit concurrency to 10 to avoid ESI rate-limit issues.
	type fetchResult struct {
		orders []esi.MarketOrder
		err    error
	}
	results := make(map[regionType]fetchResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	undercutSem := make(chan struct{}, 10) // limit to 10 concurrent ESI requests

	for pair := range pairs {
		wg.Add(1)
		go func(rt regionType) {
			defer wg.Done()
			undercutSem <- struct{}{}
			ro, fetchErr := s.esi.FetchRegionOrdersByType(rt.regionID, rt.typeID)
			<-undercutSem
			mu.Lock()
			results[rt] = fetchResult{ro, fetchErr}
			mu.Unlock()
		}(pair)
	}
	wg.Wait()

	// Flatten all regional orders into one slice.
	var allRegional []esi.MarketOrder
	for _, fr := range results {
		if fr.err == nil {
			allRegional = append(allRegional, fr.orders...)
		}
	}

	undercuts := engine.AnalyzeUndercuts(orders, allRegional)
	writeJSON(w, undercuts)
}

func (s *Server) handleAuthGetStationTradeStates(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.db == nil {
		writeJSON(w, map[string]interface{}{
			"tab":    "station",
			"states": []db.TradeState{},
		})
		return
	}

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "station"
	}
	currentRevision := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("current_revision")); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, 400, "invalid current_revision")
			return
		}
		currentRevision = parsed
	}
	pruned := int64(0)
	if currentRevision > 0 {
		n, err := s.db.DeleteExpiredDoneTradeStatesForUser(userID, tab, currentRevision)
		if err != nil {
			writeError(w, 500, "failed to prune expired trade states")
			return
		}
		pruned = n
	}

	states, err := s.db.ListTradeStatesForUser(userID, tab)
	if err != nil {
		writeError(w, 500, "failed to list trade states")
		return
	}
	if states == nil {
		states = []db.TradeState{}
	}
	writeJSON(w, map[string]interface{}{
		"tab":    tab,
		"pruned": pruned,
		"states": states,
	})
}

func (s *Server) handleAuthSetStationTradeState(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		Tab           string `json:"tab"`
		TypeID        int32  `json:"type_id"`
		StationID     int64  `json:"station_id"`
		RegionID      int32  `json:"region_id"`
		Mode          string `json:"mode"`
		UntilRevision int64  `json:"until_revision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != db.TradeStateModeDone && mode != db.TradeStateModeIgnored {
		writeError(w, 400, "mode must be done or ignored")
		return
	}
	if req.TypeID <= 0 || req.StationID <= 0 {
		writeError(w, 400, "type_id and station_id are required")
		return
	}
	if mode == db.TradeStateModeDone && req.UntilRevision <= 0 {
		req.UntilRevision = time.Now().UTC().Unix()
	}
	if mode == db.TradeStateModeIgnored {
		req.UntilRevision = 0
	}

	err := s.db.UpsertTradeStateForUser(userID, db.TradeState{
		Tab:           req.Tab,
		TypeID:        req.TypeID,
		StationID:     req.StationID,
		RegionID:      req.RegionID,
		Mode:          mode,
		UntilRevision: req.UntilRevision,
	})
	if err != nil {
		writeError(w, 500, "failed to save trade state")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok": true,
	})
}

func (s *Server) handleAuthDeleteStationTradeStates(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		Tab  string             `json:"tab"`
		Keys []db.TradeStateKey `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if len(req.Keys) == 0 {
		writeJSON(w, map[string]interface{}{
			"ok":      true,
			"deleted": 0,
		})
		return
	}

	if err := s.db.DeleteTradeStatesForUser(userID, req.Tab, req.Keys); err != nil {
		writeError(w, 500, "failed to delete trade states")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"deleted": len(req.Keys),
	})
}

func (s *Server) handleAuthClearStationTradeStates(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		Tab  string `json:"tab"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "" && mode != db.TradeStateModeDone && mode != db.TradeStateModeIgnored {
		writeError(w, 400, "mode must be empty, done, or ignored")
		return
	}
	deleted, err := s.db.ClearTradeStatesForUser(userID, req.Tab, mode)
	if err != nil {
		writeError(w, 500, "failed to clear trade states")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"deleted": deleted,
	})
}

func (s *Server) handleAuthListIndustryProjects(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := 100
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			writeError(w, 400, "invalid limit")
			return
		}
		limit = parsed
	}

	projects, err := s.db.ListIndustryProjectsForUser(userID, status, limit)
	if err != nil {
		writeError(w, 500, "failed to list industry projects")
		return
	}
	if projects == nil {
		projects = []db.IndustryProject{}
	}
	writeJSON(w, map[string]interface{}{
		"projects": projects,
		"count":    len(projects),
	})
}

func (s *Server) handleAuthCreateIndustryProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		Name     string      `json:"name"`
		Status   string      `json:"status"`
		Strategy string      `json:"strategy"`
		Notes    string      `json:"notes"`
		Params   interface{} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, 400, "name is required")
		return
	}

	project, err := s.db.CreateIndustryProjectForUser(userID, db.IndustryProjectCreateInput{
		Name:     req.Name,
		Status:   req.Status,
		Strategy: req.Strategy,
		Notes:    req.Notes,
		Params:   req.Params,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "name is required") {
			writeError(w, 400, err.Error())
			return
		}
		writeError(w, 500, "failed to create industry project")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"project": project,
	})
}

func (s *Server) handleAuthIndustryProjectSnapshot(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	projectIDStr := strings.TrimSpace(r.PathValue("projectID"))
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil || projectID <= 0 {
		writeError(w, 400, "invalid project id")
		return
	}

	snapshot, err := s.db.GetIndustryProjectSnapshotForUser(userID, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "project not found") {
			writeError(w, 404, "industry project not found")
			return
		}
		writeError(w, 500, "failed to get industry project snapshot")
		return
	}
	writeJSON(w, snapshot)
}

func (s *Server) handleAuthPreviewIndustryProjectPlan(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	projectIDStr := strings.TrimSpace(r.PathValue("projectID"))
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil || projectID <= 0 {
		writeError(w, 400, "invalid project id")
		return
	}

	var patch db.IndustryPlanPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	preview, err := s.db.PreviewIndustryPlanForUser(userID, projectID, patch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "project not found") {
			writeError(w, 404, "industry project not found")
			return
		}
		writeError(w, 500, "failed to preview industry plan")
		return
	}
	writeJSON(w, preview)
}

func (s *Server) handleAuthPlanIndustryProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	projectIDStr := strings.TrimSpace(r.PathValue("projectID"))
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil || projectID <= 0 {
		writeError(w, 400, "invalid project id")
		return
	}

	var patch db.IndustryPlanPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	summary, err := s.db.ApplyIndustryPlanForUser(userID, projectID, patch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "project not found") {
			writeError(w, 404, "industry project not found")
			return
		}
		writeError(w, 500, "failed to apply industry plan")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"summary": summary,
	})
}

func (s *Server) handleAuthRebalanceIndustryProjectMaterials(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	projectIDStr := strings.TrimSpace(r.PathValue("projectID"))
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil || projectID <= 0 {
		writeError(w, 400, "invalid project id")
		return
	}

	var req struct {
		Scope          string  `json:"scope"`
		CharacterID    int64   `json:"character_id"`
		LookbackDays   int     `json:"lookback_days"`
		Strategy       string  `json:"strategy"`
		WarehouseScope string  `json:"warehouse_scope"`
		LocationIDs    []int64 `json:"location_ids"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, 400, "invalid json")
			return
		}
	}

	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	if scope == "" {
		scope = "single"
	}
	if scope != "single" && scope != "all" {
		writeError(w, 400, "scope must be single or all")
		return
	}
	allScope := scope == "all"
	if allScope && req.CharacterID > 0 {
		writeError(w, 400, "character_id and scope=all cannot be combined")
		return
	}

	selectedSessions, err := s.authSessionsForScope(userID, req.CharacterID, allScope, true)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}

	lookbackDays := req.LookbackDays
	if lookbackDays <= 0 {
		lookbackDays = 180
	}
	if lookbackDays > 365 {
		lookbackDays = 365
	}

	strategy := strings.ToLower(strings.TrimSpace(req.Strategy))
	if strategy == "" {
		strategy = "preserve"
	}
	if strategy != "preserve" && strategy != "buy" && strategy != "build" {
		writeError(w, 400, "strategy must be preserve, buy, or build")
		return
	}

	warehouseScope := strings.ToLower(strings.TrimSpace(req.WarehouseScope))
	if warehouseScope == "" {
		warehouseScope = "location_first"
	}
	if warehouseScope != "global" && warehouseScope != "location_first" && warehouseScope != "strict_location" {
		writeError(w, 400, "warehouse_scope must be global, location_first, or strict_location")
		return
	}

	locationFilter := make(map[int64]struct{}, len(req.LocationIDs))
	for _, locationID := range req.LocationIDs {
		if locationID <= 0 {
			continue
		}
		locationFilter[locationID] = struct{}{}
	}

	fetchTxns := func(sess *auth.Session) ([]esi.WalletTransaction, error) {
		if cached, ok := s.getWalletTxnCache(sess.CharacterID); ok {
			return cached, nil
		}
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			return nil, tokenErr
		}
		freshTxns, fetchErr := s.esi.GetWalletTransactions(sess.CharacterID, token)
		if fetchErr != nil {
			return nil, fetchErr
		}

		// Enrich type names from SDE so open-position aggregation has stable labels in downstream UI/debugging.
		s.mu.RLock()
		sdeData := s.sdeData
		s.mu.RUnlock()
		if sdeData != nil {
			for i := range freshTxns {
				if t, ok := sdeData.Types[freshTxns[i].TypeID]; ok {
					freshTxns[i].TypeName = t.Name
				}
			}
		}
		s.setWalletTxnCache(sess.CharacterID, freshTxns)
		return freshTxns, nil
	}

	var txns []esi.WalletTransaction
	for _, sess := range selectedSessions {
		part, fetchErr := fetchTxns(sess)
		if fetchErr != nil {
			log.Printf("[AUTH] Industry material rebalance txns error (%s): %v", sess.CharacterName, fetchErr)
			if !allScope {
				writeError(w, 500, "failed to fetch transactions: "+fetchErr.Error())
				return
			}
			continue
		}
		txns = append(txns, part...)
	}
	if len(txns) == 0 && len(selectedSessions) > 0 {
		if allScope {
			writeError(w, 500, "failed to fetch transactions for selected characters")
		} else {
			writeError(w, 500, "failed to fetch transactions")
		}
		return
	}

	openPositions := make([]engine.OpenPosition, 0)
	if pnl := engine.ComputePortfolioPnL(txns, lookbackDays); pnl != nil && len(pnl.OpenPositions) > 0 {
		openPositions = pnl.OpenPositions
	}

	stockByType := make(map[int32]int64, len(openPositions))
	stockByTypeLocation := make(map[int32]map[int64]int64, len(openPositions))
	stockLocations := make(map[int64]struct{}, 32)
	var stockUnits int64
	positionsUsed := 0
	for _, pos := range openPositions {
		if pos.Quantity <= 0 {
			continue
		}
		if len(locationFilter) > 0 {
			if _, ok := locationFilter[pos.LocationID]; !ok {
				continue
			}
		}
		stockByType[pos.TypeID] += pos.Quantity
		if pos.LocationID > 0 {
			byLocation := stockByTypeLocation[pos.TypeID]
			if byLocation == nil {
				byLocation = make(map[int64]int64, 4)
				stockByTypeLocation[pos.TypeID] = byLocation
			}
			byLocation[pos.LocationID] += pos.Quantity
			stockLocations[pos.LocationID] = struct{}{}
		}
		stockUnits += pos.Quantity
		positionsUsed++
	}

	materials, err := s.db.RebalanceIndustryProjectMaterialsFromStockForUser(
		userID,
		projectID,
		stockByType,
		stockByTypeLocation,
		warehouseScope,
		strategy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "industry project not found")
			return
		}
		writeError(w, 500, "failed to rebalance industry materials")
		return
	}

	var availableQty int64
	var missingQty int64
	for _, row := range materials {
		availableQty += row.AvailableQty
		covered := row.AvailableQty + row.BuyQty + row.BuildQty
		missing := row.RequiredQty - covered
		if missing > 0 {
			missingQty += missing
		}
	}

	writeJSON(w, map[string]interface{}{
		"ok":        true,
		"materials": materials,
		"summary": map[string]interface{}{
			"project_id":            projectID,
			"updated":               len(materials),
			"scope":                 scope,
			"lookback_days":         lookbackDays,
			"strategy":              strategy,
			"warehouse_scope":       warehouseScope,
			"transactions":          len(txns),
			"positions_total":       len(openPositions),
			"positions_used":        positionsUsed,
			"stock_types":           len(stockByType),
			"stock_locations":       len(stockLocations),
			"stock_units":           stockUnits,
			"allocated_available":   availableQty,
			"remaining_missing_qty": missingQty,
			"location_filter_count": len(locationFilter),
		},
	})
}

func (s *Server) handleAuthSyncIndustryProjectBlueprintPool(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	projectIDStr := strings.TrimSpace(r.PathValue("projectID"))
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil || projectID <= 0 {
		writeError(w, 400, "invalid project id")
		return
	}

	var req struct {
		Scope          string  `json:"scope"`
		CharacterID    int64   `json:"character_id"`
		LocationIDs    []int64 `json:"location_ids"`
		DefaultBPCRuns int64   `json:"default_bpc_runs"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, 400, "invalid json")
			return
		}
	}

	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	if scope == "" {
		scope = "single"
	}
	if scope != "single" && scope != "all" {
		writeError(w, 400, "scope must be single or all")
		return
	}
	allScope := scope == "all"
	if allScope && req.CharacterID > 0 {
		writeError(w, 400, "character_id and scope=all cannot be combined")
		return
	}

	defaultBPCRuns := req.DefaultBPCRuns
	if defaultBPCRuns <= 0 {
		defaultBPCRuns = 1
	}
	if defaultBPCRuns > 1000 {
		defaultBPCRuns = 1000
	}

	selectedSessions, err := s.authSessionsForScope(userID, req.CharacterID, allScope, true)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}

	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()
	if sdeData == nil || sdeData.Industry == nil {
		writeError(w, 503, "industry data not ready")
		return
	}

	locationFilter := make(map[int64]struct{}, len(req.LocationIDs))
	for _, locationID := range req.LocationIDs {
		if locationID <= 0 {
			continue
		}
		locationFilter[locationID] = struct{}{}
	}

	type bpKey struct {
		TypeID     int32
		LocationID int64
		IsBPO      bool
	}

	aggregated := make(map[bpKey]db.IndustryBlueprintPoolInput, 256)
	charactersUsed := 0
	blueprintsEndpointCharacters := 0
	assetsFallbackCharacters := 0
	blueprintRowsScanned := 0
	assetsScanned := 0
	extraWarnings := make([]string, 0, 4)
	assetsFallbackWarnAdded := false
	assetResolverWarnAdded := false

	resolveRootLocationID := func(locationID int64, assetByItemID map[int64]esi.CharacterAsset) int64 {
		if locationID <= 0 || len(assetByItemID) == 0 {
			return locationID
		}
		current := locationID
		seen := map[int64]struct{}{}
		for current > 0 {
			if _, ok := seen[current]; ok {
				return current
			}
			seen[current] = struct{}{}

			parent, ok := assetByItemID[current]
			if !ok {
				return current
			}
			parentType := strings.ToLower(strings.TrimSpace(parent.LocationType))
			if parentType != "item" {
				if parent.LocationID > 0 {
					return parent.LocationID
				}
				return current
			}
			current = parent.LocationID
		}
		return locationID
	}

	upsertBlueprintPoolRow := func(typeID int32, locationID int64, isBPO bool, quantity int64, availableRuns int64, me int32, te int32) {
		if typeID <= 0 {
			return
		}
		if _, ok := sdeData.Industry.Blueprints[typeID]; !ok {
			return
		}
		if quantity <= 0 {
			quantity = 1
		}
		if !isBPO {
			if availableRuns <= 0 {
				availableRuns = quantity * defaultBPCRuns
			}
			if availableRuns < quantity {
				availableRuns = quantity
			}
		} else {
			availableRuns = 0
		}
		if len(locationFilter) > 0 {
			if _, ok := locationFilter[locationID]; !ok {
				return
			}
		}

		typeName := fmt.Sprintf("Type %d", typeID)
		if t, ok := sdeData.Types[typeID]; ok && strings.TrimSpace(t.Name) != "" {
			typeName = strings.TrimSpace(t.Name)
		}

		key := bpKey{
			TypeID:     typeID,
			LocationID: locationID,
			IsBPO:      isBPO,
		}
		row := aggregated[key]
		if row.BlueprintTypeID == 0 {
			row.BlueprintTypeID = typeID
			row.BlueprintName = typeName
			row.LocationID = locationID
			row.IsBPO = isBPO
		}
		row.Quantity += quantity
		if !isBPO {
			row.AvailableRuns += availableRuns
		}
		if me > row.ME {
			row.ME = me
		}
		if te > row.TE {
			row.TE = te
		}
		aggregated[key] = row
	}

	for _, sess := range selectedSessions {
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			log.Printf("[AUTH] Industry blueprint sync token error (%s): %v", sess.CharacterName, tokenErr)
			if !allScope {
				writeError(w, 401, tokenErr.Error())
				return
			}
			continue
		}

		sourceOK := false

		charBlueprints, bpErr := s.esi.GetCharacterBlueprints(sess.CharacterID, token)
		if bpErr == nil {
			sourceOK = true
			blueprintsEndpointCharacters++
			blueprintRowsScanned += len(charBlueprints)

			assetByItemID := map[int64]esi.CharacterAsset{}
			assets, assetErr := s.esi.GetCharacterAssets(sess.CharacterID, token)
			if assetErr == nil {
				assetsScanned += len(assets)
				assetByItemID = make(map[int64]esi.CharacterAsset, len(assets))
				for _, asset := range assets {
					if asset.ItemID > 0 {
						assetByItemID[asset.ItemID] = asset
					}
				}
			} else if !assetResolverWarnAdded {
				extraWarnings = append(extraWarnings, "blueprint location resolver unavailable: using raw location_id for some rows")
				assetResolverWarnAdded = true
			}

			for _, bp := range charBlueprints {
				if bp.TypeID <= 0 {
					continue
				}
				resolvedLocationID := resolveRootLocationID(bp.LocationID, assetByItemID)
				quantity := bp.Quantity
				if quantity <= 0 {
					quantity = 1
				}

				isBPO := bp.Runs < 0
				availableRuns := int64(0)
				if !isBPO {
					runsPerCopy := bp.Runs
					if runsPerCopy <= 0 {
						runsPerCopy = defaultBPCRuns
					}
					availableRuns = runsPerCopy * quantity
				}

				upsertBlueprintPoolRow(
					bp.TypeID,
					resolvedLocationID,
					isBPO,
					quantity,
					availableRuns,
					bp.MaterialEfficiency,
					bp.TimeEfficiency,
				)
			}
		} else {
			log.Printf("[AUTH] Industry blueprint sync blueprints error (%s): %v", sess.CharacterName, bpErr)

			assets, fetchErr := s.esi.GetCharacterAssets(sess.CharacterID, token)
			if fetchErr != nil {
				log.Printf("[AUTH] Industry blueprint sync assets fallback error (%s): %v", sess.CharacterName, fetchErr)
				if !allScope {
					writeError(w, 500, "failed to fetch blueprints/assets: "+fetchErr.Error())
					return
				}
				continue
			}

			sourceOK = true
			assetsFallbackCharacters++
			assetsScanned += len(assets)
			if !assetsFallbackWarnAdded {
				extraWarnings = append(extraWarnings, "blueprints endpoint unavailable for some characters; assets fallback used (ME/TE/runs are estimated)")
				assetsFallbackWarnAdded = true
			}

			assetByItemID := make(map[int64]esi.CharacterAsset, len(assets))
			for _, asset := range assets {
				if asset.ItemID > 0 {
					assetByItemID[asset.ItemID] = asset
				}
			}

			for _, asset := range assets {
				if asset.TypeID <= 0 {
					continue
				}
				resolvedLocationID := resolveRootLocationID(asset.LocationID, assetByItemID)
				isBPO := true
				if asset.IsBlueprintCopy || asset.Quantity <= -2 {
					isBPO = false
				}
				quantity := asset.Quantity
				if quantity <= 0 {
					quantity = 1
				}

				upsertBlueprintPoolRow(
					asset.TypeID,
					resolvedLocationID,
					isBPO,
					quantity,
					quantity*defaultBPCRuns,
					0,
					0,
				)
			}
		}

		if sourceOK {
			charactersUsed++
		}
	}

	if len(selectedSessions) > 0 && charactersUsed == 0 {
		if allScope {
			writeError(w, 500, "failed to fetch blueprints/assets for selected characters")
		} else {
			writeError(w, 500, "failed to fetch blueprints/assets")
		}
		return
	}

	blueprints := make([]db.IndustryBlueprintPoolInput, 0, len(aggregated))
	for _, row := range aggregated {
		blueprints = append(blueprints, row)
	}
	sort.SliceStable(blueprints, func(i, j int) bool {
		if blueprints[i].BlueprintTypeID != blueprints[j].BlueprintTypeID {
			return blueprints[i].BlueprintTypeID < blueprints[j].BlueprintTypeID
		}
		if blueprints[i].LocationID != blueprints[j].LocationID {
			return blueprints[i].LocationID < blueprints[j].LocationID
		}
		if blueprints[i].IsBPO == blueprints[j].IsBPO {
			return blueprints[i].BlueprintName < blueprints[j].BlueprintName
		}
		return blueprints[i].IsBPO && !blueprints[j].IsBPO
	})

	planSummary, err := s.db.ApplyIndustryPlanForUser(userID, projectID, db.IndustryPlanPatch{
		ReplaceBlueprintPool: true,
		Blueprints:           blueprints,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "project not found") {
			writeError(w, 404, "industry project not found")
			return
		}
		writeError(w, 500, "failed to sync industry blueprints")
		return
	}

	combinedWarnings := make([]string, 0, len(planSummary.Warnings)+len(extraWarnings))
	seenWarnings := make(map[string]struct{}, len(planSummary.Warnings)+len(extraWarnings))
	appendWarning := func(msg string) {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			return
		}
		if _, ok := seenWarnings[msg]; ok {
			return
		}
		seenWarnings[msg] = struct{}{}
		combinedWarnings = append(combinedWarnings, msg)
	}
	for _, msg := range planSummary.Warnings {
		appendWarning(msg)
	}
	for _, msg := range extraWarnings {
		appendWarning(msg)
	}

	writeJSON(w, map[string]interface{}{
		"ok": true,
		"summary": map[string]interface{}{
			"project_id":                     projectID,
			"scope":                          scope,
			"characters":                     len(selectedSessions),
			"characters_used":                charactersUsed,
			"blueprints_endpoint_characters": blueprintsEndpointCharacters,
			"assets_fallback_characters":     assetsFallbackCharacters,
			"blueprint_rows_scanned":         blueprintRowsScanned,
			"assets_scanned":                 assetsScanned,
			"blueprints_detected":            len(blueprints),
			"blueprints_upserted":            planSummary.BlueprintsUpsert,
			"default_bpc_runs":               defaultBPCRuns,
			"location_filter_count":          len(locationFilter),
			"warnings":                       combinedWarnings,
		},
	})
}

func (s *Server) handleAuthUpdateIndustryTaskStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		TaskID   int64  `json:"task_id"`
		Status   string `json:"status"`
		Priority *int   `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if req.TaskID <= 0 {
		writeError(w, 400, "task_id is required")
		return
	}

	task, err := s.db.UpdateIndustryTaskStatusForUser(userID, req.TaskID, req.Status, req.Priority)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "industry task not found")
			return
		}
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "invalid task status") || strings.Contains(errMsg, "must be positive") {
			writeError(w, 400, err.Error())
			return
		}
		writeError(w, 500, "failed to update industry task status")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":   true,
		"task": task,
	})
}

func (s *Server) handleAuthBulkUpdateIndustryTaskStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		TaskIDs  []int64 `json:"task_ids"`
		Status   string  `json:"status"`
		Priority *int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if len(req.TaskIDs) == 0 {
		writeError(w, 400, "task_ids are required")
		return
	}

	tasks, err := s.db.UpdateIndustryTaskStatusesForUser(userID, req.TaskIDs, req.Status, req.Priority)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "one or more industry tasks not found")
			return
		}
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "invalid task status") ||
			strings.Contains(errMsg, "must be positive") ||
			strings.Contains(errMsg, "required") {
			writeError(w, 400, err.Error())
			return
		}
		writeError(w, 500, "failed to bulk update industry task status")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"updated": len(tasks),
		"tasks":   tasks,
	})
}

func (s *Server) handleAuthUpdateIndustryTaskPriority(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		TaskID   int64 `json:"task_id"`
		Priority *int  `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if req.TaskID <= 0 {
		writeError(w, 400, "task_id is required")
		return
	}
	if req.Priority == nil {
		writeError(w, 400, "priority is required")
		return
	}

	tasks, err := s.db.UpdateIndustryTaskPrioritiesForUser(userID, []int64{req.TaskID}, *req.Priority)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "industry task not found")
			return
		}
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "must be positive") || strings.Contains(errMsg, "required") {
			writeError(w, 400, err.Error())
			return
		}
		writeError(w, 500, "failed to update industry task priority")
		return
	}
	var task interface{}
	if len(tasks) > 0 {
		task = tasks[0]
	}
	writeJSON(w, map[string]interface{}{
		"ok":   true,
		"task": task,
	})
}

func (s *Server) handleAuthBulkUpdateIndustryTaskPriority(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		TaskIDs  []int64 `json:"task_ids"`
		Priority *int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if len(req.TaskIDs) == 0 {
		writeError(w, 400, "task_ids are required")
		return
	}
	if req.Priority == nil {
		writeError(w, 400, "priority is required")
		return
	}

	tasks, err := s.db.UpdateIndustryTaskPrioritiesForUser(userID, req.TaskIDs, *req.Priority)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "one or more industry tasks not found")
			return
		}
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "must be positive") || strings.Contains(errMsg, "required") {
			writeError(w, 400, err.Error())
			return
		}
		writeError(w, 500, "failed to bulk update industry task priority")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"updated": len(tasks),
		"tasks":   tasks,
	})
}

func (s *Server) handleAuthUpdateIndustryJobStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		JobID      int64  `json:"job_id"`
		Status     string `json:"status"`
		StartedAt  string `json:"started_at"`
		FinishedAt string `json:"finished_at"`
		Notes      string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if req.JobID <= 0 {
		writeError(w, 400, "job_id is required")
		return
	}

	job, err := s.db.UpdateIndustryJobStatusForUser(userID, req.JobID, req.Status, req.StartedAt, req.FinishedAt, req.Notes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "industry job not found")
			return
		}
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "invalid job status") ||
			strings.Contains(errMsg, "must be positive") ||
			strings.Contains(errMsg, "rfc3339") {
			writeError(w, 400, err.Error())
			return
		}
		writeError(w, 500, "failed to update industry job status")
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":  true,
		"job": job,
	})
}

func (s *Server) handleAuthBulkUpdateIndustryJobStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	var req struct {
		JobIDs     []int64 `json:"job_ids"`
		Status     string  `json:"status"`
		StartedAt  string  `json:"started_at"`
		FinishedAt string  `json:"finished_at"`
		Notes      string  `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if len(req.JobIDs) == 0 {
		writeError(w, 400, "job_ids are required")
		return
	}

	jobs, err := s.db.UpdateIndustryJobStatusesForUser(
		userID,
		req.JobIDs,
		req.Status,
		req.StartedAt,
		req.FinishedAt,
		req.Notes,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "one or more industry jobs not found")
			return
		}
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "invalid job status") ||
			strings.Contains(errMsg, "must be positive") ||
			strings.Contains(errMsg, "required") ||
			strings.Contains(errMsg, "rfc3339") {
			writeError(w, 400, err.Error())
			return
		}
		writeError(w, 500, "failed to bulk update industry job status")
		return
	}

	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"updated": len(jobs),
		"jobs":    jobs,
	})
}

func (s *Server) handleAuthIndustryLedger(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireIndustryAuthUser(w, r)
	if !ok {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}

	limit := 200
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			writeError(w, 400, "invalid limit")
			return
		}
		limit = parsed
	}

	projectID := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("project_id")); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil || parsed <= 0 {
			writeError(w, 400, "invalid project_id")
			return
		}
		projectID = parsed
	}

	ledger, err := s.db.GetIndustryLedgerForUser(userID, db.IndustryLedgerOptions{
		ProjectID: projectID,
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:     limit,
	})
	if err != nil {
		writeError(w, 500, "failed to get industry ledger")
		return
	}
	writeJSON(w, ledger)
}

func (s *Server) handleAuthRebootStationCache(w http.ResponseWriter, r *http.Request) {
	if s.esi == nil {
		writeError(w, 503, "esi client unavailable")
		return
	}
	cleared := s.esi.ClearOrderCache()
	log.Printf("[API] Station cache reboot requested: cleared %d entries", cleared)
	writeJSON(w, map[string]interface{}{
		"ok":          true,
		"cleared":     cleared,
		"rebooted_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAuthOrderDesk(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, true)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}

	salesTax := 8.0
	if cfg := s.loadConfigForUser(userID); cfg != nil {
		salesTax = cfg.SalesTaxPercent
	}
	if v := r.URL.Query().Get("sales_tax"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 100 {
			salesTax = f
		}
	}
	brokerFee := 1.0
	if v := r.URL.Query().Get("broker_fee"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 100 {
			brokerFee = f
		}
	}
	targetETADays := 3.0
	if v := r.URL.Query().Get("target_eta_days"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 60 {
			targetETADays = f
		}
	}

	var orders []esi.CharacterOrder
	for _, sess := range selectedSessions {
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			log.Printf("[AUTH] OrderDesk token error (%s): %v", sess.CharacterName, tokenErr)
			if !allScope {
				writeError(w, 401, tokenErr.Error())
				return
			}
			continue
		}
		charOrders, fetchErr := s.esi.GetCharacterOrders(sess.CharacterID, token)
		if fetchErr != nil {
			log.Printf("[AUTH] OrderDesk orders error (%s): %v", sess.CharacterName, fetchErr)
			if !allScope {
				writeError(w, 500, "failed to fetch orders: "+fetchErr.Error())
				return
			}
			continue
		}
		orders = append(orders, charOrders...)
	}

	if len(orders) == 0 {
		writeJSON(w, engine.ComputeOrderDesk(nil, nil, nil, nil, engine.OrderDeskOptions{
			SalesTaxPercent:  salesTax,
			BrokerFeePercent: brokerFee,
			TargetETADays:    targetETADays,
			WarnExpiryDays:   2,
		}))
		return
	}

	// Enrich names for UI readability.
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()
	if sdeData != nil {
		locationIDs := make(map[int64]bool, len(orders))
		for _, o := range orders {
			locationIDs[o.LocationID] = true
		}
		s.esi.PrefetchStationNames(locationIDs)
		for i := range orders {
			if t, ok := sdeData.Types[orders[i].TypeID]; ok {
				orders[i].TypeName = t.Name
			}
			orders[i].LocationName = s.esi.StationName(orders[i].LocationID)
		}
	}

	type regionType struct {
		regionID int32
		typeID   int32
	}
	pairs := make(map[regionType]bool)
	for _, o := range orders {
		pairs[regionType{regionID: o.RegionID, typeID: o.TypeID}] = true
	}

	type fetchResult struct {
		orders []esi.MarketOrder
		err    error
	}
	books := make(map[regionType]fetchResult)
	history := make(map[engine.OrderDeskHistoryKey][]esi.HistoryEntry)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for pair := range pairs {
		wg.Add(1)
		go func(rt regionType) {
			defer wg.Done()

			sem <- struct{}{}
			ro, fetchErr := s.esi.FetchRegionOrdersByType(rt.regionID, rt.typeID)
			<-sem

			var entries []esi.HistoryEntry
			var ok bool
			if s.db != nil {
				entries, ok = s.db.GetMarketHistory(rt.regionID, rt.typeID)
			}
			if !ok {
				fresh, histErr := s.esi.FetchMarketHistory(rt.regionID, rt.typeID)
				if histErr == nil {
					entries = fresh
					if s.db != nil && len(entries) > 0 {
						s.db.SetMarketHistory(rt.regionID, rt.typeID, entries)
					}
				}
			}

			mu.Lock()
			books[rt] = fetchResult{orders: ro, err: fetchErr}
			if len(entries) > 0 {
				history[engine.NewOrderDeskHistoryKey(rt.regionID, rt.typeID)] = entries
			}
			mu.Unlock()
		}(pair)
	}
	wg.Wait()

	var allRegional []esi.MarketOrder
	unavailableBooks := make(map[engine.OrderDeskHistoryKey]bool)
	for rt, fr := range books {
		if fr.err == nil {
			allRegional = append(allRegional, fr.orders...)
			continue
		}
		unavailableBooks[engine.NewOrderDeskHistoryKey(rt.regionID, rt.typeID)] = true
	}

	result := engine.ComputeOrderDesk(orders, allRegional, history, unavailableBooks, engine.OrderDeskOptions{
		SalesTaxPercent:  salesTax,
		BrokerFeePercent: brokerFee,
		TargetETADays:    targetETADays,
		WarnExpiryDays:   2,
	})
	writeJSON(w, result)
}

func (s *Server) handleAuthStationCommand(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	var req struct {
		StationID            int64   `json:"station_id"` // 0 = all stations
		RegionID             int32   `json:"region_id"`
		SystemName           string  `json:"system_name"`
		IgnoredSystemIDs     []int32 `json:"ignored_system_ids"`
		Radius               int     `json:"radius"`
		MinMargin            float64 `json:"min_margin"`
		SalesTaxPercent      float64 `json:"sales_tax_percent"`
		BrokerFee            float64 `json:"broker_fee"`
		CTSProfile           string  `json:"cts_profile"`
		SplitTradeFees       bool    `json:"split_trade_fees"`
		BuyBrokerFeePercent  float64 `json:"buy_broker_fee_percent"`
		SellBrokerFeePercent float64 `json:"sell_broker_fee_percent"`
		BuySalesTaxPercent   float64 `json:"buy_sales_tax_percent"`
		SellSalesTaxPercent  float64 `json:"sell_sales_tax_percent"`
		MinDailyVolume       int64   `json:"min_daily_volume"`
		MinItemProfit        float64 `json:"min_item_profit"`
		MinDemandPerDay      float64 `json:"min_demand_per_day"` // legacy alias for min_s2b_per_day
		MinS2BPerDay         float64 `json:"min_s2b_per_day"`
		MinBfSPerDay         float64 `json:"min_bfs_per_day"`
		AvgPricePeriod       int     `json:"avg_price_period"`
		MinPeriodROI         float64 `json:"min_period_roi"`
		BvSRatioMin          float64 `json:"bvs_ratio_min"`
		BvSRatioMax          float64 `json:"bvs_ratio_max"`
		MaxPVI               float64 `json:"max_pvi"`
		MaxSDS               int     `json:"max_sds"`
		LimitBuyToPriceLow   bool    `json:"limit_buy_to_price_low"`
		FlagExtremePrices    bool    `json:"flag_extreme_prices"`
		IncludeStructures    bool    `json:"include_structures"`
		StructureIDs         []int64 `json:"structure_ids"`
		TargetETADays        float64 `json:"target_eta_days"`
		LookbackDays         int     `json:"lookback_days"`
		MaxResults           int     `json:"max_results"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, true)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}

	s.mu.RLock()
	scanner := s.scanner
	sdeData := s.sdeData
	s.mu.RUnlock()
	if scanner == nil || sdeData == nil {
		writeError(w, 503, "station scanner not ready")
		return
	}

	stationIDs := make(map[int64]bool)
	regionIDs := make(map[int32]bool)
	allowedSystemsByRegion := make(map[int32]map[int32]bool)
	ignoredSystems := ignoredSystemSet(sdeData.Systems, req.IgnoredSystemIDs)
	historyLabel := ""
	radiusMode := req.Radius > 0 && req.SystemName != ""
	singleStationMode := !radiusMode && req.StationID > 0
	allStationsMode := !radiusMode && !singleStationMode

	if radiusMode {
		systemID, ok := sdeData.SystemByName[strings.ToLower(req.SystemName)]
		if !ok {
			writeError(w, 400, "unknown system")
			return
		}
		systems := sdeData.Universe.SystemsWithinRadius(systemID, req.Radius)
		for ignoredID := range ignoredSystems {
			delete(systems, ignoredID)
		}
		for _, st := range sdeData.Stations {
			if _, inRange := systems[st.SystemID]; inRange {
				stationIDs[st.ID] = true
			}
		}
		for sysID := range systems {
			if sys, ok := sdeData.Systems[sysID]; ok {
				regionIDs[sys.RegionID] = true
				sysSet, exists := allowedSystemsByRegion[sys.RegionID]
				if !exists {
					sysSet = make(map[int32]bool)
					allowedSystemsByRegion[sys.RegionID] = sysSet
				}
				sysSet[sysID] = true
			}
		}
		historyLabel = fmt.Sprintf("%s +%d jumps", req.SystemName, req.Radius)
	} else if singleStationMode {
		if st, ok := sdeData.Stations[req.StationID]; !(ok && ignoredSystems[st.SystemID]) {
			stationIDs[req.StationID] = true
		}
		regionIDs[req.RegionID] = true
		historyLabel = fmt.Sprintf("Station %d", req.StationID)
	} else {
		regionIDs[req.RegionID] = true
		historyLabel = fmt.Sprintf("Region %d (all)", req.RegionID)
	}

	if req.IncludeStructures && len(req.StructureIDs) > 0 && !allStationsMode {
		for _, sid := range req.StructureIDs {
			stationIDs[sid] = true
		}
	}

	accessToken := ""
	if req.IncludeStructures && s.sessions != nil {
		if token, tokenErr := s.sessions.EnsureValidTokenForUser(s.sso, userID); tokenErr == nil {
			accessToken = token
		}
	}

	userCfg := s.loadConfigForUser(userID)
	if !req.SplitTradeFees {
		if req.SalesTaxPercent <= 0 && userCfg != nil && userCfg.SalesTaxPercent > 0 {
			req.SalesTaxPercent = userCfg.SalesTaxPercent
		}
		if req.BrokerFee <= 0 && userCfg != nil && userCfg.BrokerFeePercent > 0 {
			req.BrokerFee = userCfg.BrokerFeePercent
		}
	}

	var scanResults []engine.StationTrade
	for regionID := range regionIDs {
		if err := r.Context().Err(); err != nil {
			writeError(w, 499, "request canceled")
			return
		}
		params := engine.StationTradeParams{
			StationIDs:           stationIDs,
			AllowedSystems:       allowedSystemsByRegion[regionID],
			IgnoredSystems:       ignoredSystems,
			RegionID:             regionID,
			MinMargin:            req.MinMargin,
			SalesTaxPercent:      req.SalesTaxPercent,
			BrokerFee:            req.BrokerFee,
			CTSProfile:           req.CTSProfile,
			SplitTradeFees:       req.SplitTradeFees,
			BuyBrokerFeePercent:  req.BuyBrokerFeePercent,
			SellBrokerFeePercent: req.SellBrokerFeePercent,
			BuySalesTaxPercent:   req.BuySalesTaxPercent,
			SellSalesTaxPercent:  req.SellSalesTaxPercent,
			MinDailyVolume:       req.MinDailyVolume,
			MinItemProfit:        req.MinItemProfit,
			MinDemandPerDay:      req.MinDemandPerDay,
			MinS2BPerDay:         req.MinS2BPerDay,
			MinBfSPerDay:         req.MinBfSPerDay,
			AvgPricePeriod:       req.AvgPricePeriod,
			MinPeriodROI:         req.MinPeriodROI,
			BvSRatioMin:          req.BvSRatioMin,
			BvSRatioMax:          req.BvSRatioMax,
			MaxPVI:               req.MaxPVI,
			MaxSDS:               req.MaxSDS,
			LimitBuyToPriceLow:   req.LimitBuyToPriceLow,
			FlagExtremePrices:    req.FlagExtremePrices,
			AccessToken:          accessToken,
			IncludeStructures:    req.IncludeStructures,
			Ctx:                  r.Context(),
		}
		if allStationsMode {
			params.StationIDs = nil
		}

		results, scanErr := scanner.ScanStationTrades(params, func(string) {})
		if scanErr != nil {
			if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
				writeError(w, 499, "request canceled")
				return
			}
			writeError(w, 500, scanErr.Error())
			return
		}
		scanResults = append(scanResults, results...)
	}

	if !req.IncludeStructures {
		scanResults = filterStationTradesExcludeStructures(scanResults)
	}
	scanResults = filterStationTradesMarketDisabled(scanResults)
	sort.Slice(scanResults, func(i, j int) bool {
		if scanResults[i].CTS != scanResults[j].CTS {
			return scanResults[i].CTS > scanResults[j].CTS
		}
		return scanResults[i].DailyProfit > scanResults[j].DailyProfit
	})
	if req.MaxResults > 0 && len(scanResults) > req.MaxResults {
		scanResults = scanResults[:req.MaxResults]
	}

	var activeOrders []esi.CharacterOrder
	for _, sess := range selectedSessions {
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			log.Printf("[AUTH] StationCommand token error (%s): %v", sess.CharacterName, tokenErr)
			if !allScope {
				writeError(w, 401, tokenErr.Error())
				return
			}
			continue
		}
		charOrders, fetchErr := s.esi.GetCharacterOrders(sess.CharacterID, token)
		if fetchErr != nil {
			log.Printf("[AUTH] StationCommand orders error (%s): %v", sess.CharacterName, fetchErr)
			if !allScope {
				writeError(w, 500, "failed to fetch orders: "+fetchErr.Error())
				return
			}
			continue
		}
		activeOrders = append(activeOrders, charOrders...)
	}

	if len(activeOrders) > 0 {
		locationIDs := make(map[int64]bool, len(activeOrders))
		for _, o := range activeOrders {
			locationIDs[o.LocationID] = true
		}
		s.esi.PrefetchStationNames(locationIDs)
		for i := range activeOrders {
			if t, ok := sdeData.Types[activeOrders[i].TypeID]; ok {
				activeOrders[i].TypeName = t.Name
			}
			activeOrders[i].LocationName = s.esi.StationName(activeOrders[i].LocationID)
		}
	}

	type regionType struct {
		regionID int32
		typeID   int32
	}
	pairs := make(map[regionType]bool)
	for _, o := range activeOrders {
		pairs[regionType{regionID: o.RegionID, typeID: o.TypeID}] = true
	}

	type fetchResult struct {
		orders []esi.MarketOrder
		err    error
	}
	bookByPair := make(map[regionType]fetchResult)
	var booksMu sync.Mutex
	var booksWG sync.WaitGroup
	booksSem := make(chan struct{}, 10)

	for pair := range pairs {
		booksWG.Add(1)
		go func(rt regionType) {
			defer booksWG.Done()
			booksSem <- struct{}{}
			ro, fetchErr := s.esi.FetchRegionOrdersByType(rt.regionID, rt.typeID)
			<-booksSem
			booksMu.Lock()
			bookByPair[rt] = fetchResult{orders: ro, err: fetchErr}
			booksMu.Unlock()
		}(pair)
	}
	booksWG.Wait()

	var allRegional []esi.MarketOrder
	for _, fr := range bookByPair {
		if fr.err == nil {
			allRegional = append(allRegional, fr.orders...)
		}
	}

	salesTax := req.SalesTaxPercent
	if salesTax <= 0 {
		if userCfg != nil && userCfg.SalesTaxPercent > 0 {
			salesTax = userCfg.SalesTaxPercent
		} else {
			salesTax = 8.0
		}
	}
	brokerFee := req.BrokerFee
	if brokerFee <= 0 {
		if userCfg != nil && userCfg.BrokerFeePercent > 0 {
			brokerFee = userCfg.BrokerFeePercent
		} else {
			brokerFee = 1.0
		}
	}
	targetETADays := req.TargetETADays
	if targetETADays <= 0 {
		targetETADays = 3.0
	}
	orderDesk := engine.ComputeOrderDesk(activeOrders, allRegional, nil, nil, engine.OrderDeskOptions{
		SalesTaxPercent:  salesTax,
		BrokerFeePercent: brokerFee,
		TargetETADays:    targetETADays,
		WarnExpiryDays:   2,
	})

	lookbackDays := req.LookbackDays
	if lookbackDays <= 0 {
		lookbackDays = 180
	}
	if lookbackDays > 365 {
		lookbackDays = 365
	}

	var txns []esi.WalletTransaction
	for _, sess := range selectedSessions {
		if !allScope {
			if cached, ok := s.getWalletTxnCache(sess.CharacterID); ok {
				txns = append(txns, cached...)
				continue
			}
		}
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			log.Printf("[AUTH] StationCommand tx token error (%s): %v", sess.CharacterName, tokenErr)
			if !allScope {
				writeError(w, 401, tokenErr.Error())
				return
			}
			continue
		}
		freshTxns, fetchErr := s.esi.GetWalletTransactions(sess.CharacterID, token)
		if fetchErr != nil {
			log.Printf("[AUTH] StationCommand tx fetch error (%s): %v", sess.CharacterName, fetchErr)
			if !allScope {
				writeError(w, 500, "failed to fetch wallet transactions: "+fetchErr.Error())
				return
			}
			continue
		}
		if !allScope {
			s.setWalletTxnCache(sess.CharacterID, freshTxns)
		}
		txns = append(txns, freshTxns...)
	}

	openPositions := make([]engine.OpenPosition, 0)
	if pnl := engine.ComputePortfolioPnL(txns, lookbackDays); pnl != nil && len(pnl.OpenPositions) > 0 {
		openPositions = pnl.OpenPositions
	}
	command := engine.BuildStationCommand(scanResults, activeOrders, openPositions)

	var openQtyTotal int64
	for _, pos := range openPositions {
		openQtyTotal += pos.Quantity
	}

	response := struct {
		GeneratedAt string                      `json:"generated_at"`
		Scope       string                      `json:"scope"`
		ScanScope   string                      `json:"scan_scope"`
		RegionCount int                         `json:"region_count"`
		ResultCount int                         `json:"result_count"`
		CacheMeta   stationCacheMeta            `json:"cache_meta"`
		Command     engine.StationCommandResult `json:"command"`
		OrderDesk   engine.OrderDeskResponse    `json:"order_desk"`
		Inventory   struct {
			OpenPositions int   `json:"open_positions"`
			OpenQuantity  int64 `json:"open_quantity"`
			Transactions  int   `json:"transactions"`
		} `json:"inventory"`
	}{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Scope: func() string {
			if allScope {
				return "all"
			}
			return "single"
		}(),
		ScanScope:   historyLabel,
		RegionCount: len(regionIDs),
		ResultCount: len(scanResults),
		CacheMeta:   s.stationCacheMetaForRegions(regionIDs),
		Command:     command,
		OrderDesk:   orderDesk,
	}
	response.Inventory.OpenPositions = len(openPositions)
	response.Inventory.OpenQuantity = openQtyTotal
	response.Inventory.Transactions = len(txns)

	writeJSON(w, response)
}

func containsAnyLower(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func stationAIIsDiagnosticAssistantMessage(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}
	if containsAnyLower(lower, []string{
		"need_full_context",
		"constraint_violation",
		"rows_seen_count",
		"preflight",
		`"status":"need_full_context"`,
		`"status": "need_full_context"`,
		`"status":"constraint_violation"`,
		`"status": "constraint_violation"`,
	}) {
		return true
	}
	// Drop assistant JSON diagnostics to reduce prompt state contamination.
	if strings.HasPrefix(lower, "{") && strings.HasSuffix(lower, "}") &&
		containsAnyLower(lower, []string{`"status"`, `"rows_count"`, `"runtime_available"`}) {
		return true
	}
	return false
}

func normalizeStationAIHistory(history []stationAIHistoryMessage) []stationAIHistoryMessage {
	if len(history) == 0 {
		return nil
	}
	out := make([]stationAIHistoryMessage, 0, len(history))
	for _, msg := range history {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if role == "assistant" && stationAIIsDiagnosticAssistantMessage(content) {
			continue
		}
		runes := []rune(content)
		if len(runes) > 1800 {
			content = string(runes[:1800]) + "..."
		}
		out = append(out, stationAIHistoryMessage{
			Role:    role,
			Content: content,
		})
	}
	if len(out) > stationAIHistoryMaxMessages {
		out = out[len(out)-stationAIHistoryMaxMessages:]
	}
	return out
}

func normalizeStationAIChatRequest(req *stationAIChatRequestPayload) (bool, bool, []string, string) {
	req.Provider = strings.TrimSpace(strings.ToLower(req.Provider))
	if req.Provider == "" {
		req.Provider = "openrouter"
	}
	if req.Provider != "openrouter" {
		return false, false, nil, "unsupported ai provider"
	}
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.APIKey == "" {
		return false, false, nil, "api_key is required"
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		return false, false, nil, "model is required"
	}
	req.PlannerModel = strings.TrimSpace(req.PlannerModel)
	req.UserMessage = strings.TrimSpace(req.UserMessage)
	if req.UserMessage == "" {
		return false, false, nil, "user_message is required"
	}

	req.Locale = strings.TrimSpace(strings.ToLower(req.Locale))
	if req.Locale != "ru" && req.Locale != "en" {
		req.Locale = "en"
	}
	req.AssistantName = strings.TrimSpace(req.AssistantName)
	if req.AssistantName == "" {
		req.AssistantName = "Ivy AI"
	}

	enableWiki := true
	if req.EnableWiki != nil {
		enableWiki = *req.EnableWiki
	}
	enableWeb := false
	if req.EnableWeb != nil {
		enableWeb = *req.EnableWeb
	}
	req.WikiRepo = sanitizeWikiRepo(req.WikiRepo)
	req.History = normalizeStationAIHistory(req.History)

	warnings := make([]string, 0, 8)
	if req.Temperature < 0 || req.Temperature > 2 {
		req.Temperature = 0.2
		warnings = append(warnings, "temperature clamped to 0.2")
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 900
	}
	if req.MaxTokens > stationAIMaxTokensLimit {
		req.MaxTokens = stationAIMaxTokensLimit
		warnings = append(warnings, "max_tokens clamped to 1000000")
	}
	if len(req.Context.Rows) > 100 {
		req.Context.Rows = req.Context.Rows[:100]
		warnings = append(warnings, "rows context was truncated to 100")
	}

	req.Context.ScanSnapshot.ScopeMode = strings.TrimSpace(strings.ToLower(req.Context.ScanSnapshot.ScopeMode))
	switch req.Context.ScanSnapshot.ScopeMode {
	case "", "radius", "single_station", "region_all":
		// allowed
	default:
		req.Context.ScanSnapshot.ScopeMode = ""
		warnings = append(warnings, "scan_snapshot.scope_mode reset to empty")
	}

	req.Context.ScanSnapshot.CTSProfile = strings.TrimSpace(strings.ToLower(req.Context.ScanSnapshot.CTSProfile))
	switch req.Context.ScanSnapshot.CTSProfile {
	case "", "balanced", "aggressive", "defensive":
		// allowed
	default:
		req.Context.ScanSnapshot.CTSProfile = "balanced"
		warnings = append(warnings, "scan_snapshot.cts_profile reset to balanced")
	}

	if req.Context.ScanSnapshot.StructureCount < 0 {
		req.Context.ScanSnapshot.StructureCount = 0
	}
	if len(req.Context.ScanSnapshot.StructureIDs) > 300 {
		req.Context.ScanSnapshot.StructureIDs = req.Context.ScanSnapshot.StructureIDs[:300]
		warnings = append(warnings, "scan_snapshot.structure_ids was truncated to 300")
	}
	// Runtime account context is backend-owned and must not be accepted from client JSON.
	req.Context.Runtime = nil
	return enableWiki, enableWeb, warnings, ""
}

func detectStationAIIntent(userMessage string, history []stationAIHistoryMessage) stationAIIntentKind {
	msg := strings.TrimSpace(strings.ToLower(userMessage))
	if msg == "" {
		return stationAIIntentGeneral
	}

	debugTerms := []string{
		"error", "bug", "trace", "stack", "undefined", "null", "crash", "failed", "panic",
		"ошибка", "баг", "сломал", "сломалось", "не работает", "краш", "исключение", "лог",
	}
	if containsAnyLower(msg, debugTerms) {
		return stationAIIntentDebug
	}

	strongTradingTerms := []string{
		"scan_snapshot", "decision matrix", "execute now", "monitor bucket", "capital allocation",
		"parameter patch", "stress test", "risk_score", "rows", "runtime", "summary",
		"min_daily_volume", "min_item_profit", "min_margin", "bvs_ratio", "max_pvi", "max_sds",
		"матриц", "капитал", "стресс", "параметр патч", "риск_скор", "скан_снапшот",
	}
	if containsAnyLower(msg, strongTradingTerms) {
		return stationAIIntentTrading
	}

	productTerms := []string{
		"wiki", "documentation", "docs", "roadmap", "project", "feature", "api",
		"док", "документац", "проект", "роадмап", "фича", "как работает",
	}
	if containsAnyLower(msg, productTerms) {
		return stationAIIntentProduct
	}

	webTerms := []string{
		"google", "search web", "internet", "latest", "today", "news", "reddit", "forum",
		"гугл", "погугл", "интернет", "сегодня", "новости", "свеж", "внешн",
	}
	if containsAnyLower(msg, webTerms) {
		return stationAIIntentResearch
	}

	tradingTerms := []string{
		"trade", "trading", "scan", "station", "profit", "margin", "risk", "filters",
		"actionable", "reprice", "order", "orders", "liquidity", "volume", "eta", "cts",
		"сделк", "трейд", "скан", "станц", "прибыл", "марж", "риск", "фильтр", "ордер",
		"ликвид", "объем", "объём", "действ", "спред",
	}
	if containsAnyLower(msg, tradingTerms) {
		return stationAIIntentTrading
	}

	greetingTerms := []string{
		"hi", "hello", "hey", "yo", "sup", "thanks", "thank you",
		"привет", "здравствуй", "добрый день", "добрый вечер", "ку", "спасибо",
	}
	if utf8.RuneCountInString(msg) <= 40 && containsAnyLower(msg, greetingTerms) {
		return stationAIIntentSmallTalk
	}

	followupTerms := []string{
		"почему", "объясни", "что дальше", "дальше", "why", "explain", "next",
	}
	if containsAnyLower(msg, followupTerms) {
		for i := len(history) - 1; i >= 0 && i >= len(history)-4; i-- {
			if history[i].Role != "assistant" {
				continue
			}
			prev := strings.ToLower(history[i].Content)
			if containsAnyLower(prev, []string{"recommendation", "рекомендац", "risk", "риск", "filter", "фильтр", "trade", "трейд"}) {
				return stationAIIntentTrading
			}
		}
	}

	return stationAIIntentGeneral
}

func stationAIDefaultPlannerPlan(intent stationAIIntentKind) stationAIPlannerPlan {
	plan := stationAIPlannerPlan{
		Intent:       intent,
		ContextLevel: "summary",
		ResponseMode: "qa",
		NeedWiki:     false,
		NeedWeb:      false,
		Agents:       []string{"intent_router"},
	}
	switch intent {
	case stationAIIntentSmallTalk:
		plan.ContextLevel = "none"
		plan.ResponseMode = "short"
	case stationAIIntentTrading:
		plan.ContextLevel = "full"
		plan.ResponseMode = "structured"
		plan.Agents = []string{"scan_analyzer", "risk_checker"}
	case stationAIIntentProduct:
		plan.ContextLevel = "summary"
		plan.ResponseMode = "qa"
		plan.NeedWiki = true
		plan.Agents = []string{"wiki_retriever"}
	case stationAIIntentDebug:
		plan.ContextLevel = "summary"
		plan.ResponseMode = "diagnostic"
		plan.NeedWiki = true
		plan.Agents = []string{"debug_helper", "wiki_retriever"}
	case stationAIIntentResearch:
		plan.ContextLevel = "summary"
		plan.ResponseMode = "qa"
		plan.NeedWeb = true
		plan.Agents = []string{"web_retriever"}
	default:
		plan.ContextLevel = "summary"
		plan.ResponseMode = "qa"
	}
	return plan
}

func stationAINormalizePlannerPlan(plan stationAIPlannerPlan, fallback stationAIPlannerPlan) stationAIPlannerPlan {
	out := fallback
	switch plan.Intent {
	case stationAIIntentSmallTalk, stationAIIntentTrading, stationAIIntentProduct, stationAIIntentDebug, stationAIIntentResearch, stationAIIntentGeneral:
		out.Intent = plan.Intent
	}
	switch strings.TrimSpace(strings.ToLower(plan.ContextLevel)) {
	case "none", "summary", "full":
		out.ContextLevel = strings.TrimSpace(strings.ToLower(plan.ContextLevel))
	}
	switch strings.TrimSpace(strings.ToLower(plan.ResponseMode)) {
	case "short", "structured", "diagnostic", "qa":
		out.ResponseMode = strings.TrimSpace(strings.ToLower(plan.ResponseMode))
	}
	out.NeedWiki = plan.NeedWiki
	out.NeedWeb = plan.NeedWeb
	out.AskClarification = plan.AskClarification
	out.Clarification = strings.TrimSpace(plan.Clarification)
	if len([]rune(out.Clarification)) > 280 {
		out.Clarification = string([]rune(out.Clarification)[:280]) + "..."
	}
	if len(plan.Agents) > 0 {
		agents := make([]string, 0, len(plan.Agents))
		seen := map[string]struct{}{}
		allowed := map[string]struct{}{
			"intent_router":  {},
			"scan_analyzer":  {},
			"risk_checker":   {},
			"wiki_retriever": {},
			"web_retriever":  {},
			"debug_helper":   {},
		}
		for _, a := range plan.Agents {
			agent := strings.TrimSpace(strings.ToLower(a))
			if agent == "" {
				continue
			}
			if _, ok := allowed[agent]; !ok {
				continue
			}
			if _, ok := seen[agent]; ok {
				continue
			}
			seen[agent] = struct{}{}
			agents = append(agents, agent)
			if len(agents) >= 6 {
				break
			}
		}
		if len(agents) > 0 {
			out.Agents = agents
		}
	}
	// Guardrails for smalltalk: no heavy retrieval by default.
	if out.Intent == stationAIIntentSmallTalk {
		out.ContextLevel = "none"
		out.NeedWiki = false
		out.NeedWeb = false
		out.ResponseMode = "short"
	}
	// Trading analysis needs row-level context by default; summary-only responses
	// produce generic answers and miss actionable opportunities.
	if out.Intent == stationAIIntentTrading {
		out.ContextLevel = "full"
		if len(out.Agents) == 0 {
			out.Agents = []string{"scan_analyzer", "risk_checker"}
		}
	}
	return out
}

func aiExtractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return ""
	}
	return raw[start : end+1]
}

func stationAIPlannerContextSnippet(ctx stationAIContextPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "system=%s; station_scope=%s; rows=%d; actionable=%d; avg_margin=%.2f; avg_daily_profit=%.2f\n",
		ctx.SystemName,
		ctx.StationScope,
		ctx.Summary.VisibleRows,
		ctx.Summary.ActionableRows,
		ctx.Summary.AverageMargin,
		ctx.Summary.AverageProfit,
	)
	if ctx.ScanSnapshot.ScopeMode != "" {
		fmt.Fprintf(
			&b,
			"scan_snapshot: scope=%s; split_fees=%t; cts=%s; min_margin=%.2f; min_daily_volume=%d; min_item_profit=%.2f; min_s2b=%.2f; min_bfs=%.2f; avg_price_period=%d; min_period_roi=%.2f; bvs_ratio=[%.2f..%.2f]; max_pvi=%.2f; max_sds=%.2f; limit_buy_to_low=%t; flag_extreme=%t; include_structures=%t (applied=%t count=%d)\n",
			ctx.ScanSnapshot.ScopeMode,
			ctx.ScanSnapshot.SplitTradeFees,
			ctx.ScanSnapshot.CTSProfile,
			ctx.ScanSnapshot.MinMargin,
			ctx.ScanSnapshot.MinDailyVolume,
			ctx.ScanSnapshot.MinItemProfit,
			ctx.ScanSnapshot.MinS2BPerDay,
			ctx.ScanSnapshot.MinBfSPerDay,
			ctx.ScanSnapshot.AvgPricePeriod,
			ctx.ScanSnapshot.MinPeriodROI,
			ctx.ScanSnapshot.BVSRatioMin,
			ctx.ScanSnapshot.BVSRatioMax,
			ctx.ScanSnapshot.MaxPVI,
			ctx.ScanSnapshot.MaxSDS,
			ctx.ScanSnapshot.LimitBuyToPriceLow,
			ctx.ScanSnapshot.FlagExtremePrices,
			ctx.ScanSnapshot.IncludeStructures,
			ctx.ScanSnapshot.StructuresApplied,
			ctx.ScanSnapshot.StructureCount,
		)
	}
	limit := len(ctx.Rows)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		row := ctx.Rows[i]
		fmt.Fprintf(&b, "- %s | action=%s | margin=%.2f | daily_profit=%.2f | risk=%t\n",
			row.TypeName, row.Action, row.Margin, row.DailyProfit, row.HighRisk)
	}
	return b.String()
}

func stationAIPlannerSystemPrompt(locale string) string {
	if locale == "ru" {
		return "Ты planner-агент для EVE Flipper. Твоя задача: определить намерение пользователя и выбрать минимально нужный контекст/агентов для следующего LLM шага. Верни ТОЛЬКО JSON без markdown. Поля JSON: intent, context_level, response_mode, need_wiki, need_web, ask_clarification, clarification, agents. intent one of: smalltalk|trading_analysis|product_help|debug_support|web_research|general. context_level one of: none|summary|full. response_mode one of: short|structured|diagnostic|qa."
	}
	return "You are the planner agent for EVE Flipper. Determine user intent and choose minimal required context/agents for the next LLM step. Return JSON ONLY, no markdown. JSON fields: intent, context_level, response_mode, need_wiki, need_web, ask_clarification, clarification, agents. intent one of: smalltalk|trading_analysis|product_help|debug_support|web_research|general. context_level one of: none|summary|full. response_mode one of: short|structured|diagnostic|qa."
}

func stationAIPlannerUserPrompt(locale, userMessage string, history []stationAIHistoryMessage, ctx stationAIContextPayload, fallback stationAIPlannerPlan) string {
	var hb strings.Builder
	if len(history) > 0 {
		start := 0
		if len(history) > 6 {
			start = len(history) - 6
		}
		for i := start; i < len(history); i++ {
			msg := history[i]
			fmt.Fprintf(&hb, "[%s] %s\n", msg.Role, aiTrimForPrompt(msg.Content, 220))
		}
	}
	if locale == "ru" {
		return fmt.Sprintf(
			"Сообщение пользователя:\n%s\n\n"+
				"Краткая история:\n%s\n"+
				"Сводка контекста таба:\n%s\n"+
				"Fallback intent: %s\n"+
				"Верни только JSON.",
			userMessage,
			hb.String(),
			stationAIPlannerContextSnippet(ctx),
			fallback.Intent,
		)
	}
	return fmt.Sprintf(
		"User message:\n%s\n\n"+
			"Recent history:\n%s\n"+
			"Tab context summary:\n%s\n"+
			"Fallback intent: %s\n"+
			"Return JSON only.",
		userMessage,
		hb.String(),
		stationAIPlannerContextSnippet(ctx),
		fallback.Intent,
	)
}

func stationAIContextForPlan(ctx stationAIContextPayload, plan stationAIPlannerPlan) stationAIContextPayload {
	out := stationAIContextForIntent(ctx, plan.Intent)
	switch plan.ContextLevel {
	case "none":
		out.Rows = nil
		out.Summary = stationAIContextSummary{}
	case "summary":
		out.Rows = nil
	case "full":
		if len(out.Rows) > 100 {
			out.Rows = out.Rows[:100]
		}
	default:
		// keep intent-based default
	}
	return out
}

func stationAIPreflight(locale string, plan stationAIPlannerPlan, ctx stationAIContextPayload, runtimeRequested bool) stationAIPreflightResult {
	result := stationAIPreflightResult{Status: "pass"}
	requireRows := plan.ContextLevel == "full" || plan.Intent == stationAIIntentTrading
	if requireRows && len(ctx.Rows) == 0 {
		result.Status = "fail"
		result.Missing = append(result.Missing, "rows")
	}
	if plan.Intent == stationAIIntentTrading && ctx.Summary.VisibleRows == 0 && len(ctx.Rows) == 0 {
		result.Status = "fail"
		result.Missing = append(result.Missing, "summary.visible_rows")
	}
	if runtimeRequested && (ctx.Runtime == nil || !ctx.Runtime.Available) {
		result.Caveats = append(result.Caveats, stationAIRuntimeLocaleText(
			locale,
			"account runtime context unavailable; response quality may be lower",
			"runtime-контекст аккаунта недоступен; качество ответа может быть ниже",
		))
	}
	if result.Status == "pass" && len(result.Caveats) > 0 {
		result.Status = "partial"
	}
	return result
}

func stationAIPreflightCaveatBlock(locale string, preflight stationAIPreflightResult) string {
	if len(preflight.Caveats) == 0 {
		return ""
	}
	if locale == "ru" {
		return "Ограничения контекста: " + strings.Join(preflight.Caveats, "; ")
	}
	return "Context caveats: " + strings.Join(preflight.Caveats, "; ")
}

func stationAIPreflightFailAnswer(locale string, missing []string) string {
	list := strings.Join(missing, ", ")
	if list == "" {
		list = "context"
	}
	if locale == "ru" {
		return "Не могу выполнить этот анализ: не хватает обязательных данных контекста (" + list + "). Перезапусти скан и повтори запрос."
	}
	return "I cannot run this analysis because required context fields are missing (" + list + "). Re-run scan and try again."
}

func stationAIValidateAnswer(answer string, intent stationAIIntentKind) (bool, string) {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return false, "empty ai answer"
	}
	lower := strings.ToLower(trimmed)
	if containsAnyLower(lower, []string{
		"need_full_context",
		"constraint_violation",
		"rows_seen_count",
		`"status":"need_full_context"`,
		`"status": "need_full_context"`,
		`"status":"constraint_violation"`,
		`"status": "constraint_violation"`,
		"preflight",
	}) {
		return false, "diagnostic markers leaked into answer"
	}
	if intent == stationAIIntentTrading {
		if utf8.RuneCountInString(trimmed) < 60 {
			return false, "trading answer too short"
		}
		hasDigit := false
		for _, r := range trimmed {
			if unicode.IsDigit(r) {
				hasDigit = true
				break
			}
		}
		if !hasDigit {
			return false, "trading answer has no numeric evidence"
		}
	}
	return true, ""
}

func stationAIRetryCorrectionPrompt(locale, issue string) string {
	issue = strings.TrimSpace(issue)
	if issue == "" {
		issue = "invalid answer format"
	}
	if locale == "ru" {
		return fmt.Sprintf(
			"Предыдущий ответ отклонен серверной валидацией: %s. "+
				"Дай прямой аналитический ответ по текущему контексту. "+
				"Не выполняй self-check/preflight и не выводи диагностические маркеры.",
			issue,
		)
	}
	return fmt.Sprintf(
		"Your previous response was rejected by server validation: %s. "+
			"Provide a direct analytical answer using current context only. "+
			"Do not run self-check/preflight and do not output diagnostic markers.",
		issue,
	)
}

func stationAIUsageTokenInts(usage map[string]interface{}) (int, int, int) {
	if usage == nil {
		return 0, 0, 0
	}
	readInt := func(key string) int {
		raw, ok := usage[key]
		if !ok {
			return 0
		}
		switch v := raw.(type) {
		case int:
			return v
		case int32:
			return int(v)
		case int64:
			return int(v)
		case float32:
			return int(v)
		case float64:
			return int(v)
		default:
			return 0
		}
	}
	return readInt("prompt_tokens"), readInt("completion_tokens"), readInt("total_tokens")
}

func stationAIPlannerEnabled(req stationAIChatRequestPayload) bool {
	if req.EnablePlanner != nil {
		return *req.EnablePlanner
	}
	return true
}

func stationAIResolvePlannerModel(req stationAIChatRequestPayload) string {
	model := strings.TrimSpace(req.PlannerModel)
	if model != "" {
		return model
	}
	if strings.TrimSpace(defaultStationAIPlannerModel) != "" {
		return strings.TrimSpace(defaultStationAIPlannerModel)
	}
	return strings.TrimSpace(req.Model)
}

func stationAIPipelineMeta(req stationAIChatRequestPayload, plan stationAIPlannerPlan, plannerEnabled bool) map[string]interface{} {
	return map[string]interface{}{
		"planner_enabled": plannerEnabled,
		"planner_model":   stationAIResolvePlannerModel(req),
		"response_mode":   plan.ResponseMode,
		"context_level":   plan.ContextLevel,
		"agents":          plan.Agents,
	}
}

func (s *Server) stationAIResolvePlan(ctx context.Context, req stationAIChatRequestPayload) (stationAIPlannerPlan, bool, []string) {
	intent := detectStationAIIntent(req.UserMessage, req.History)
	plan := stationAIDefaultPlannerPlan(intent)
	if stationAINeedsWikiContext(intent, req.UserMessage) {
		plan.NeedWiki = true
	}
	if stationAINeedsWebResearch(intent, req.UserMessage) {
		plan.NeedWeb = true
	}
	plannerEnabled := stationAIPlannerEnabled(req)
	if !plannerEnabled {
		return plan, false, nil
	}
	planned, warnings := s.stationAIPlannerPass(ctx, req, plan)
	return planned, true, warnings
}

func (s *Server) stationAIPlannerPass(ctx context.Context, req stationAIChatRequestPayload, fallback stationAIPlannerPlan) (stationAIPlannerPlan, []string) {
	model := stationAIResolvePlannerModel(req)

	payload := map[string]interface{}{
		"model":       model,
		"temperature": 0.0,
		"max_tokens":  220,
		"messages": []map[string]string{
			{"role": "system", "content": stationAIPlannerSystemPrompt(req.Locale)},
			{
				"role": "user",
				"content": stationAIPlannerUserPrompt(
					req.Locale,
					req.UserMessage,
					req.History,
					req.Context,
					fallback,
				),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fallback, []string{"planner: failed to encode planner request"}
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://openrouter.ai/api/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return fallback, []string{"planner: failed to create planner request"}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("HTTP-Referer", "http://localhost:1420")
	httpReq.Header.Set("X-Title", "EVE Flipper Station AI Planner")

	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fallback, []string{"planner unavailable, using fallback intent routing"}
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if err != nil {
		return fallback, []string{"planner read failed, using fallback intent routing"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fallback, []string{"planner provider error, using fallback intent routing"}
	}

	var orResp struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawResp, &orResp); err != nil || len(orResp.Choices) == 0 {
		return fallback, []string{"planner invalid response, using fallback intent routing"}
	}
	content := extractAIContent(orResp.Choices[0].Message.Content)
	jsonBlock := aiExtractJSONObject(content)
	if jsonBlock == "" {
		return fallback, []string{"planner did not return json, using fallback intent routing"}
	}

	var plan stationAIPlannerPlan
	if err := json.Unmarshal([]byte(jsonBlock), &plan); err != nil {
		return fallback, []string{"planner json parse failed, using fallback intent routing"}
	}
	normalized := stationAINormalizePlannerPlan(plan, fallback)
	return normalized, nil
}

func stationAIContextForIntent(ctx stationAIContextPayload, intent stationAIIntentKind) stationAIContextPayload {
	out := ctx
	switch intent {
	case stationAIIntentSmallTalk:
		out.Rows = nil
		out.Summary = stationAIContextSummary{}
	case stationAIIntentProduct, stationAIIntentResearch:
		out.Rows = nil
	case stationAIIntentGeneral:
		if len(out.Rows) > 12 {
			out.Rows = out.Rows[:12]
		}
	}
	return out
}

func stationAINeedsWikiContext(intent stationAIIntentKind, userMessage string) bool {
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	if intent == stationAIIntentSmallTalk {
		return false
	}
	if containsAnyLower(msg, []string{"wiki", "docs", "documentation", "док", "документац", "проект"}) {
		return true
	}
	return intent == stationAIIntentProduct || intent == stationAIIntentDebug
}

func stationAINeedsWebResearch(intent stationAIIntentKind, userMessage string) bool {
	if intent == stationAIIntentSmallTalk {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	if intent == stationAIIntentResearch {
		return true
	}
	return containsAnyLower(msg, []string{
		"latest", "today", "news", "reddit", "forum", "search", "web",
		"сегодня", "новости", "свеж", "погугл", "интернет", "внешн",
	})
}

func stationAIRuntimeLocaleText(locale, en, ru string) string {
	if locale == "ru" {
		return ru
	}
	return en
}

func stationAINeedsRuntimeContext(intent stationAIIntentKind, userMessage string) bool {
	if intent == stationAIIntentSmallTalk || intent == stationAIIntentProduct {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	if msg == "" {
		return false
	}

	// Hard triggers for account/runtime analytics.
	if containsAnyLower(msg, []string{
		"wallet", "balance", "capital", "budget", "portfolio", "pnl", "realized",
		"transaction", "transactions", "ledger",
		"кошел", "баланс", "капитал", "бюджет", "портфел", "пнл", "реализован",
		"транзак", "журнал сделок", "история сделок",
	}) {
		return true
	}

	// Soft triggers: only with personal scope to avoid heavy fetches on generic questions.
	personalTerms := []string{"my ", "mine", "for me", "мой", "моя", "мои", "моё", "у меня", "для меня"}
	softTerms := []string{
		"order", "orders", "history", "risk", "var", "es95", "es99",
		"ордер", "ордера", "истор", "риск", "математ", "формул",
	}
	return containsAnyLower(msg, personalTerms) && containsAnyLower(msg, softTerms)
}

func (s *Server) stationAIEnrichTxnTypeNames(txns []esi.WalletTransaction) {
	if len(txns) == 0 {
		return
	}
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()
	if sdeData == nil {
		return
	}
	for i := range txns {
		if strings.TrimSpace(txns[i].TypeName) != "" {
			continue
		}
		if t, ok := sdeData.Types[txns[i].TypeID]; ok {
			txns[i].TypeName = t.Name
		}
	}
}

func stationAISummarizeRuntimeTransactions(runtime *stationAIRuntimeContext, txns []esi.WalletTransaction) {
	if runtime == nil || len(txns) == 0 {
		return
	}
	windowStart := time.Now().UTC().AddDate(0, 0, -stationAIRuntimeTxnWindowDays)
	type itemAgg struct {
		typeID   int32
		typeName string
		trades   int
		volume   int64
		turnover float64
	}
	byType := make(map[int32]*itemAgg)

	for _, tx := range txns {
		ts, err := time.Parse(time.RFC3339, tx.Date)
		if err != nil || ts.Before(windowStart) {
			continue
		}
		notional := tx.UnitPrice * float64(tx.Quantity)
		runtime.TransactionCount++
		if tx.IsBuy {
			runtime.BuyFlowISK += notional
		} else {
			runtime.SellFlowISK += notional
		}

		item := byType[tx.TypeID]
		if item == nil {
			typeName := strings.TrimSpace(tx.TypeName)
			if typeName == "" {
				typeName = fmt.Sprintf("Type %d", tx.TypeID)
			}
			item = &itemAgg{
				typeID:   tx.TypeID,
				typeName: typeName,
			}
			byType[tx.TypeID] = item
		}
		item.trades++
		item.volume += int64(tx.Quantity)
		item.turnover += notional
	}

	runtime.NetFlowISK = runtime.SellFlowISK - runtime.BuyFlowISK
	if len(byType) == 0 {
		return
	}

	items := make([]itemAgg, 0, len(byType))
	for _, item := range byType {
		items = append(items, *item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].turnover == items[j].turnover {
			return items[i].trades > items[j].trades
		}
		return items[i].turnover > items[j].turnover
	})
	if len(items) > stationAIRuntimeTopItems {
		items = items[:stationAIRuntimeTopItems]
	}
	runtime.TopItems = make([]stationAIRuntimeItemFlow, 0, len(items))
	for _, item := range items {
		runtime.TopItems = append(runtime.TopItems, stationAIRuntimeItemFlow{
			TypeID:      item.typeID,
			TypeName:    item.typeName,
			Trades:      item.trades,
			Volume:      item.volume,
			TurnoverISK: item.turnover,
		})
	}
}

func (s *Server) stationAIBuildRuntimeContext(ctx context.Context, userID, locale string) (*stationAIRuntimeContext, []string) {
	runtime := &stationAIRuntimeContext{
		Available:     false,
		TxnWindowDays: stationAIRuntimeTxnWindowDays,
		FetchedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	warnings := make([]string, 0, 4)
	seenWarnings := make(map[string]struct{}, 4)
	var mu sync.Mutex
	addWarning := func(msg string) {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			return
		}
		mu.Lock()
		if _, exists := seenWarnings[msg]; exists {
			mu.Unlock()
			return
		}
		seenWarnings[msg] = struct{}{}
		warnings = append(warnings, msg)
		runtime.Notes = append(runtime.Notes, msg)
		mu.Unlock()
	}

	notLoggedWarn := stationAIRuntimeLocaleText(
		locale,
		"runtime account context unavailable: user is not logged in",
		"runtime-контекст аккаунта недоступен: пользователь не авторизован",
	)
	cancelWarn := stationAIRuntimeLocaleText(
		locale,
		"runtime account context fetch canceled",
		"сбор runtime-контекста аккаунта отменен",
	)
	if s.sessions == nil {
		addWarning(notLoggedWarn)
		return runtime, warnings
	}
	sess := s.sessions.GetForUser(userID)
	if sess == nil {
		addWarning(notLoggedWarn)
		return runtime, warnings
	}
	runtime.CharacterID = sess.CharacterID
	runtime.CharacterName = sess.CharacterName

	if err := ctx.Err(); err != nil {
		addWarning(cancelWarn)
		return runtime, warnings
	}
	token, err := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
	if err != nil {
		addWarning(stationAIRuntimeLocaleText(
			locale,
			"runtime account context unavailable: auth token refresh failed",
			"runtime-контекст аккаунта недоступен: не удалось обновить auth-токен",
		))
		return runtime, warnings
	}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		if err := ctx.Err(); err != nil {
			addWarning(cancelWarn)
			return
		}
		balance, fetchErr := s.esi.GetWalletBalance(sess.CharacterID, token)
		if fetchErr != nil {
			addWarning(stationAIRuntimeLocaleText(
				locale,
				"runtime context: failed to fetch wallet balance",
				"runtime-контекст: не удалось получить баланс кошелька",
			))
			return
		}
		mu.Lock()
		runtime.WalletAvailable = true
		runtime.WalletISK = balance
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		if err := ctx.Err(); err != nil {
			addWarning(cancelWarn)
			return
		}
		orders, fetchErr := s.esi.GetCharacterOrders(sess.CharacterID, token)
		if fetchErr != nil {
			addWarning(stationAIRuntimeLocaleText(
				locale,
				"runtime context: failed to fetch active orders",
				"runtime-контекст: не удалось получить активные ордера",
			))
			return
		}
		buyOrders := 0
		sellOrders := 0
		notional := 0.0
		for _, o := range orders {
			notional += o.Price * float64(o.VolumeRemain)
			if o.IsBuyOrder {
				buyOrders++
			} else {
				sellOrders++
			}
		}
		mu.Lock()
		runtime.OrdersAvailable = true
		runtime.ActiveOrders = len(orders)
		runtime.BuyOrders = buyOrders
		runtime.SellOrders = sellOrders
		runtime.OpenOrderNotionalISK = notional
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		if err := ctx.Err(); err != nil {
			addWarning(cancelWarn)
			return
		}
		txns, ok := s.getWalletTxnCache(sess.CharacterID)
		if !ok {
			freshTxns, fetchErr := s.esi.GetWalletTransactions(sess.CharacterID, token)
			if fetchErr != nil {
				addWarning(stationAIRuntimeLocaleText(
					locale,
					"runtime context: failed to fetch wallet transactions",
					"runtime-контекст: не удалось получить транзакции кошелька",
				))
				return
			}
			s.setWalletTxnCache(sess.CharacterID, freshTxns)
			txns = freshTxns
		}
		s.stationAIEnrichTxnTypeNames(txns)
		txSummary := stationAIRuntimeContext{
			TxnWindowDays: stationAIRuntimeTxnWindowDays,
		}
		stationAISummarizeRuntimeTransactions(&txSummary, txns)
		risk := engine.ComputePortfolioRiskFromTransactions(txns)
		mu.Lock()
		runtime.TransactionsAvailable = true
		runtime.TransactionCount = txSummary.TransactionCount
		runtime.BuyFlowISK = txSummary.BuyFlowISK
		runtime.SellFlowISK = txSummary.SellFlowISK
		runtime.NetFlowISK = txSummary.NetFlowISK
		runtime.TopItems = txSummary.TopItems
		runtime.Risk = risk
		mu.Unlock()
	}()

	wg.Wait()

	runtime.Available = runtime.WalletAvailable || runtime.OrdersAvailable || runtime.TransactionsAvailable
	if runtime.Available {
		log.Printf(
			"[AI][RUNTIME] character=%s(%d) wallet=%t orders=%t tx=%t tx_count=%d",
			runtime.CharacterName,
			runtime.CharacterID,
			runtime.WalletAvailable,
			runtime.OrdersAvailable,
			runtime.TransactionsAvailable,
			runtime.TransactionCount,
		)
	} else {
		log.Printf(
			"[AI][RUNTIME] character=%s(%d) unavailable notes=%v",
			runtime.CharacterName,
			runtime.CharacterID,
			runtime.Notes,
		)
	}

	return runtime, warnings
}

func buildStationAIMessages(systemPrompt string, history []stationAIHistoryMessage, userPrompt string) []map[string]string {
	messages := make([]map[string]string, 0, 2+len(history))
	messages = append(messages, map[string]string{
		"role":    "system",
		"content": systemPrompt,
	})
	for _, msg := range history {
		messages = append(messages, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": userPrompt,
	})
	return messages
}

func readBodyWithLimit(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("invalid max body size: %d", maxBytes)
	}
	limited := io.LimitReader(r, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxBytes)
	}
	return body, nil
}

func (s *Server) stationAIOpenRouterChatOnce(
	ctx context.Context,
	req stationAIChatRequestPayload,
	messages []map[string]string,
) (stationAIProviderReply, error) {
	payload := map[string]interface{}{
		"model":       req.Model,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"messages":    messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return stationAIProviderReply{}, fmt.Errorf("failed to encode ai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://openrouter.ai/api/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return stationAIProviderReply{}, fmt.Errorf("failed to create ai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("HTTP-Referer", "http://localhost:1420")
	httpReq.Header.Set("X-Title", "EVE Flipper Station AI")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return stationAIProviderReply{}, fmt.Errorf("ai provider request failed: %w", err)
	}
	defer resp.Body.Close()

	rawResp, readErr := readBodyWithLimit(resp.Body, stationAIProviderResponseMaxBodyBytes)
	if readErr != nil {
		return stationAIProviderReply{}, fmt.Errorf("failed to read ai provider response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := "ai provider error"
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(rawResp, &errBody) == nil && strings.TrimSpace(errBody.Error.Message) != "" {
			errMsg = strings.TrimSpace(errBody.Error.Message)
		}
		return stationAIProviderReply{}, errors.New(errMsg)
	}

	var orResp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage map[string]interface{} `json:"usage"`
	}
	if err := json.Unmarshal(rawResp, &orResp); err != nil {
		return stationAIProviderReply{}, fmt.Errorf("invalid ai provider response: %w", err)
	}
	if len(orResp.Choices) == 0 {
		return stationAIProviderReply{}, errors.New("empty ai response")
	}

	answer := strings.TrimSpace(extractAIContent(orResp.Choices[0].Message.Content))
	if answer == "" {
		return stationAIProviderReply{}, errors.New("empty ai answer")
	}
	model := strings.TrimSpace(orResp.Model)
	if model == "" {
		model = req.Model
	}
	return stationAIProviderReply{
		Answer:     answer,
		Model:      model,
		ProviderID: strings.TrimSpace(orResp.ID),
		Usage:      orResp.Usage,
	}, nil
}

func (s *Server) handleAuthStationAIChat(w http.ResponseWriter, r *http.Request) {
	var req stationAIChatRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}

	enableWiki, enableWeb, warnings, validationErr := normalizeStationAIChatRequest(&req)
	if validationErr != "" {
		writeError(w, 400, validationErr)
		return
	}

	plan, plannerEnabled, plannerWarnings := s.stationAIResolvePlan(r.Context(), req)
	warnings = append(warnings, plannerWarnings...)
	intent := plan.Intent
	useWiki := enableWiki && intent != stationAIIntentSmallTalk
	useWeb := enableWeb && intent != stationAIIntentSmallTalk
	useRuntime := stationAINeedsRuntimeContext(intent, req.UserMessage)
	pipeline := stationAIPipelineMeta(req, plan, plannerEnabled)
	log.Printf(
		"[AI][CHAT] mode=sync intent=%s planner=%t wiki_enabled=%t need_wiki=%t wiki_used=%t web_enabled=%t need_web=%t web_used=%t runtime_requested=%t locale=%s model=%s",
		intent,
		plannerEnabled,
		enableWiki,
		plan.NeedWiki,
		useWiki,
		enableWeb,
		plan.NeedWeb,
		useWeb,
		useRuntime,
		req.Locale,
		req.Model,
	)
	if len(plannerWarnings) > 0 {
		log.Printf("[AI][CHAT] mode=sync planner_warnings=%v", plannerWarnings)
	}
	if plan.AskClarification && strings.TrimSpace(plan.Clarification) != "" {
		warnings = append(warnings, "planner asked for clarification")
		writeJSON(w, map[string]interface{}{
			"answer":    strings.TrimSpace(plan.Clarification),
			"provider":  req.Provider,
			"model":     req.Model,
			"assistant": req.AssistantName,
			"intent":    string(intent),
			"pipeline":  pipeline,
			"warnings":  warnings,
		})
		return
	}
	contextForPrompt := stationAIContextForPlan(req.Context, plan)
	runtimeUsed := false
	if useRuntime {
		userID := userIDFromRequest(r)
		runtimeCtx, rw := s.stationAIBuildRuntimeContext(r.Context(), userID, req.Locale)
		if runtimeCtx != nil {
			contextForPrompt.Runtime = runtimeCtx
			runtimeUsed = runtimeCtx.Available
		}
		warnings = append(warnings, rw...)
		log.Printf("[AI][CHAT] mode=sync runtime_requested=%t runtime_used=%t", useRuntime, runtimeUsed)
	}
	preflight := stationAIPreflight(req.Locale, plan, contextForPrompt, useRuntime)
	pipeline["preflight_status"] = preflight.Status
	if len(preflight.Missing) > 0 {
		pipeline["preflight_missing"] = preflight.Missing
	}
	if len(preflight.Caveats) > 0 {
		pipeline["preflight_caveats"] = preflight.Caveats
	}
	if preflight.Status == "fail" {
		warnings = append(warnings, "preflight failed: missing "+strings.Join(preflight.Missing, ", "))
		answer := stationAIPreflightFailAnswer(req.Locale, preflight.Missing)
		writeJSON(w, map[string]interface{}{
			"answer":    answer,
			"provider":  req.Provider,
			"model":     req.Model,
			"assistant": req.AssistantName,
			"intent":    string(intent),
			"pipeline":  pipeline,
			"warnings":  warnings,
		})
		log.Printf("[AI][CHAT] mode=sync preflight=fail intent=%s missing=%v", intent, preflight.Missing)
		return
	}
	contextJSON, err := json.Marshal(contextForPrompt)
	if err != nil {
		writeError(w, 500, "failed to encode context")
		return
	}
	wikiSnippets := make([]aiKnowledgeSnippet, 0, 4)
	webSnippets := make([]aiKnowledgeSnippet, 0, 4)
	if useWiki {
		ws, ww := s.stationAIWikiSnippets(r.Context(), req.Locale, req.UserMessage, req.WikiRepo, intent)
		wikiSnippets = ws
		warnings = append(warnings, ww...)
	} else if enableWiki {
		if intent == stationAIIntentSmallTalk {
			log.Printf("[AI][CHAT] mode=sync wiki skipped for smalltalk intent")
		} else if !plan.NeedWiki {
			log.Printf("[AI][CHAT] mode=sync wiki skipped by planner intent=%s", intent)
		}
	}
	if useWeb {
		ws, ww := stationAIWebSnippets(r.Context(), req.Locale, req.UserMessage, intent)
		webSnippets = ws
		warnings = append(warnings, ww...)
	} else if enableWeb {
		log.Printf("[AI][CHAT] mode=sync web skipped for smalltalk intent")
	}
	knowledgeBlock := buildStationAIKnowledgeBlock(req.Locale, wikiSnippets, webSnippets)
	agentBlock := buildStationAIAgentBlock(req.Locale, plan, contextForPrompt, wikiSnippets, webSnippets)

	systemPrompt := stationAISystemPrompt(req.Locale, req.AssistantName, plan)
	userPrompt := stationAIUserPrompt(req.Locale, req.UserMessage, contextJSON, plan)
	if caveatBlock := stationAIPreflightCaveatBlock(req.Locale, preflight); caveatBlock != "" {
		userPrompt += "\n\n" + caveatBlock
	}
	if agentBlock != "" {
		userPrompt += "\n\n" + agentBlock
	}
	if knowledgeBlock != "" {
		userPrompt += "\n\n" + knowledgeBlock
	}
	messages := buildStationAIMessages(systemPrompt, req.History, userPrompt)

	reply, err := s.stationAIOpenRouterChatOnce(r.Context(), req, messages)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}

	if valid, issue := stationAIValidateAnswer(reply.Answer, intent); !valid {
		warnings = append(warnings, "server validation requested retry: "+issue)
		retryMessages := make([]map[string]string, 0, len(messages)+2)
		retryMessages = append(retryMessages, messages...)
		retryMessages = append(retryMessages,
			map[string]string{"role": "assistant", "content": reply.Answer},
			map[string]string{"role": "user", "content": stationAIRetryCorrectionPrompt(req.Locale, issue)},
		)
		retryReply, retryErr := s.stationAIOpenRouterChatOnce(r.Context(), req, retryMessages)
		if retryErr != nil {
			warnings = append(warnings, "retry failed: "+retryErr.Error())
		} else if validRetry, retryIssue := stationAIValidateAnswer(retryReply.Answer, intent); validRetry {
			reply = retryReply
		} else {
			warnings = append(warnings, "retry rejected: "+retryIssue)
		}
	}

	writeJSON(w, map[string]interface{}{
		"answer":         reply.Answer,
		"provider":       req.Provider,
		"model":          reply.Model,
		"assistant":      req.AssistantName,
		"intent":         string(intent),
		"pipeline":       pipeline,
		"warnings":       warnings,
		"provider_id":    reply.ProviderID,
		"provider_usage": reply.Usage,
	})
	log.Printf(
		"[AI][CHAT] mode=sync done intent=%s wiki_snippets=%d web_snippets=%d warnings=%d provider_model=%s",
		intent,
		len(wikiSnippets),
		len(webSnippets),
		len(warnings),
		reply.Model,
	)
}

func estimateTokensFromRuneCount(runes int) int {
	if runes <= 0 {
		return 0
	}
	// Heuristic for mixed EN/RU text; used only for live UI progress, not billing.
	tokens := int(float64(runes)/3.6 + 0.5)
	if tokens < 1 {
		return 1
	}
	return tokens
}

func estimateTokensFromText(text string) int {
	return estimateTokensFromRuneCount(utf8.RuneCountInString(text))
}

func sanitizeWikiRepo(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return defaultStationAIWikiRepo
	}

	// Accept git/ssh variants and plain github paths
	repo = strings.TrimPrefix(repo, "git@github.com:")
	repo = strings.TrimPrefix(repo, "github.com/")

	// Accept full URL format like https://github.com/owner/repo/wiki
	if strings.HasPrefix(strings.ToLower(repo), "http://") || strings.HasPrefix(strings.ToLower(repo), "https://") {
		u, err := url.Parse(repo)
		if err != nil {
			return defaultStationAIWikiRepo
		}
		host := strings.ToLower(strings.TrimSpace(u.Host))
		if host != "github.com" && host != "www.github.com" {
			return defaultStationAIWikiRepo
		}
		repo = strings.Trim(u.Path, "/")
	}

	segments := strings.Split(strings.Trim(repo, "/"), "/")
	if len(segments) < 2 {
		return defaultStationAIWikiRepo
	}
	owner := segments[0]
	repoName := segments[1]

	// Normalize common suffixes
	repoName = strings.TrimSuffix(repoName, ".git")
	repoName = strings.TrimSuffix(repoName, ".wiki")
	if len(segments) >= 3 {
		last := strings.ToLower(segments[len(segments)-1])
		if last == "wiki" {
			// keep owner/repo from first two segments
		}
	}

	if !aiRepoPartRe.MatchString(owner) || !aiRepoPartRe.MatchString(repoName) {
		return defaultStationAIWikiRepo
	}
	return owner + "/" + repoName
}

func aiTrimForPrompt(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	return strings.TrimSpace(text[:maxChars]) + "..."
}

func aiKeywordTerms(message string) []string {
	lower := strings.ToLower(message)
	replacer := strings.NewReplacer(
		",", " ",
		".", " ",
		":", " ",
		";", " ",
		"!", " ",
		"?", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"\"", " ",
		"'", " ",
		"\n", " ",
		"\t", " ",
		"/", " ",
		"\\", " ",
	)
	clean := replacer.Replace(lower)
	parts := strings.Fields(clean)
	if len(parts) == 0 {
		return nil
	}
	stop := map[string]struct{}{
		"что": {}, "как": {}, "где": {}, "когда": {}, "почему": {}, "для": {}, "или": {}, "это": {}, "есть": {},
		"with": {}, "from": {}, "that": {}, "this": {}, "what": {}, "when": {}, "where": {}, "which": {},
	}
	seen := make(map[string]struct{}, len(parts))
	terms := make([]string, 0, 12)
	for _, p := range parts {
		if len(p) < 3 {
			continue
		}
		if _, ok := stop[p]; ok {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		terms = append(terms, p)
		if len(terms) >= 12 {
			break
		}
	}
	return terms
}

func aiScoreAndSnippet(doc string, terms []string) (int, string) {
	if strings.TrimSpace(doc) == "" {
		return 0, ""
	}
	if len(terms) == 0 {
		return 1, aiTrimForPrompt(doc, 520)
	}
	lower := strings.ToLower(doc)
	score := 0
	firstPos := -1
	for _, term := range terms {
		pos := strings.Index(lower, term)
		if pos < 0 {
			continue
		}
		score++
		if firstPos == -1 || pos < firstPos {
			firstPos = pos
		}
	}
	if score == 0 {
		return 0, ""
	}
	start := firstPos - 220
	if start < 0 {
		start = 0
	}
	end := firstPos + 420
	if end > len(doc) {
		end = len(doc)
	}
	return score, aiTrimForPrompt(doc[start:end], 620)
}

func (s *Server) fetchAIWikiMarkdown(ctx context.Context, repo, page string) (string, string, error) {
	key := repo + "|" + page
	if cachedRaw, ok := aiWikiPageCache.Load(key); ok {
		if cached, ok2 := cachedRaw.(aiWikiCacheEntry); ok2 {
			age := time.Since(cached.FetchedAt)
			if cached.Err != "" {
				// Do not keep transient wiki errors for too long.
				if age <= aiWikiErrorCacheTTL {
					return "", cached.URL, errors.New(cached.Err)
				}
			} else if age <= aiWikiCacheTTL {
				return cached.Body, cached.URL, nil
			}
		}
	}

	pageEscaped := url.PathEscape(page)
	urlStr := fmt.Sprintf("https://raw.githubusercontent.com/wiki/%s/%s.md", repo, pageEscaped)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		aiWikiPageCache.Store(key, aiWikiCacheEntry{URL: urlStr, FetchedAt: time.Now(), Err: err.Error()})
		return "", urlStr, err
	}
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		aiWikiPageCache.Store(key, aiWikiCacheEntry{URL: urlStr, FetchedAt: time.Now(), Err: err.Error()})
		return "", urlStr, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("wiki http %d", resp.StatusCode)
		aiWikiPageCache.Store(key, aiWikiCacheEntry{URL: urlStr, FetchedAt: time.Now(), Err: err.Error()})
		return "", urlStr, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 900_000))
	if err != nil {
		aiWikiPageCache.Store(key, aiWikiCacheEntry{URL: urlStr, FetchedAt: time.Now(), Err: err.Error()})
		return "", urlStr, err
	}
	content := strings.TrimSpace(string(body))
	aiWikiPageCache.Store(key, aiWikiCacheEntry{
		Body:      content,
		URL:       urlStr,
		FetchedAt: time.Now(),
	})
	return content, urlStr, nil
}

func (s *Server) stationAIWikiSnippets(ctx context.Context, locale, userMessage, repo string, intent stationAIIntentKind) ([]aiKnowledgeSnippet, []string) {
	repo = sanitizeWikiRepo(repo)
	warnings := make([]string, 0, 3)
	if s.wikiRAG != nil {
		if ragSnippets, ragWarnings, ragErr := s.wikiRAG.Retrieve(ctx, repo, locale, userMessage, intent, stationAIWikiTopK); ragErr == nil && len(ragSnippets) > 0 {
			top := ragSnippets[0]
			log.Printf(
				"[AI][WIKI-RAG] repo=%s intent=%s locale=%s results=%d top_page=%s top_section=%s hybrid=%.4f vector=%.4f keyword=%.4f",
				repo,
				intent,
				locale,
				len(ragSnippets),
				top.Page,
				top.Section,
				top.HybridScore,
				top.VectorScore,
				top.KeywordScore,
			)
			warnings = append(warnings, ragWarnings...)
			return ragSnippets, warnings
		} else if ragErr != nil {
			log.Printf("[AI][WIKI-RAG] repo=%s intent=%s locale=%s error=%v", repo, intent, locale, ragErr)
			warnings = append(warnings, "wiki rag unavailable, using fallback retrieval")
		} else {
			log.Printf("[AI][WIKI-RAG] repo=%s intent=%s locale=%s no semantic hits, using fallback retrieval", repo, intent, locale)
		}
	}

	terms := aiKeywordTerms(userMessage)
	pages := []string{
		"Home",
		"Station-Trading",
		"Execution-Plan",
		"Radius-Scan",
		"Region-Arbitrage",
		"Route-Trading",
		"Contract-Scanner",
		"Industry-Chain-Optimizer",
		"Getting-Started",
		"API-Reference",
		"PLEX-Dashboard",
		"War-Tracker",
	}
	type wikiPageDoc struct {
		Title string
		Page  string
		URL   string
		Body  string
		Index int
	}
	docs := make([]wikiPageDoc, 0, len(pages))
	candidates := make([]aiKnowledgeSnippet, 0, 12)
	for idx, page := range pages {
		body, srcURL, err := s.fetchAIWikiMarkdown(ctx, repo, page)
		if err != nil || strings.TrimSpace(body) == "" {
			continue
		}
		title := strings.ReplaceAll(page, "-", " ")
		docs = append(docs, wikiPageDoc{
			Title: title,
			Page:  page,
			URL:   srcURL,
			Body:  body,
			Index: idx,
		})
		score, snippet := aiScoreAndSnippet(body, terms)
		if score == 0 || strings.TrimSpace(snippet) == "" {
			continue
		}
		candidates = append(candidates, aiKnowledgeSnippet{
			SourceLabel: "WIKI",
			Title:       title,
			Page:        title,
			Section:     title,
			Locale:      locale,
			URL:         srcURL,
			Content:     snippet,
			Score:       score,
		})
	}
	if len(candidates) == 0 && len(docs) > 0 {
		// Fallback mode: return generic snippets when no keyword hit is found.
		for _, doc := range docs {
			_, snippet := aiScoreAndSnippet(doc.Body, nil)
			if strings.TrimSpace(snippet) == "" {
				continue
			}
			candidates = append(candidates, aiKnowledgeSnippet{
				SourceLabel: "WIKI",
				Title:       doc.Title,
				Page:        strings.ReplaceAll(doc.Page, "-", " "),
				Section:     strings.ReplaceAll(doc.Page, "-", " "),
				Locale:      locale,
				URL:         doc.URL,
				Content:     snippet,
				Score:       100 - doc.Index,
			})
			if len(candidates) >= 4 {
				break
			}
		}
	}
	if readme, err := os.ReadFile("README.md"); err == nil {
		score, snippet := aiScoreAndSnippet(string(readme), terms)
		if score > 0 && strings.TrimSpace(snippet) != "" {
			candidates = append(candidates, aiKnowledgeSnippet{
				SourceLabel: "README",
				Title:       "README",
				Page:        "README",
				Section:     "README",
				Locale:      locale,
				URL:         "https://github.com/" + repo,
				Content:     snippet,
				Score:       score,
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Title < candidates[j].Title
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > 4 {
		candidates = candidates[:4]
	}
	log.Printf("[AI][WIKI-FALLBACK] repo=%s intent=%s locale=%s terms=%d snippets=%d", repo, intent, locale, len(terms), len(candidates))
	if len(candidates) == 0 {
		if locale == "ru" {
			warnings = append(warnings, "wiki context unavailable")
		} else {
			warnings = append(warnings, "wiki context unavailable")
		}
	}
	return candidates, warnings
}

func stationAIWebSnippets(ctx context.Context, locale, userMessage string, intent stationAIIntentKind) ([]aiKnowledgeSnippet, []string) {
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return nil, nil
	}
	unavailableWarn := "web research unavailable"
	noDataWarn := "web research returned no snippets"
	partialWarn := "web research returned partial results"
	if locale == "ru" {
		unavailableWarn = "web research недоступен"
		noDataWarn = "web research не вернул сниппетов"
		partialWarn = "web research вернул частичные результаты"
	}

	queries := stationAIWebQueryVariants(locale, userMessage, intent)
	if len(queries) == 0 {
		return nil, []string{noDataWarn}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	out := make([]aiKnowledgeSnippet, 0, stationAIWebMaxSnippets)
	seen := make(map[string]struct{}, stationAIWebMaxSnippets*2)
	hadErrors := false

	for i, query := range queries {
		remaining := stationAIWebMaxSnippets - len(out)
		if remaining <= 0 {
			break
		}
		found, err := stationAIFetchDuckDuckGoSnippets(ctx, client, query, remaining)
		if err != nil {
			hadErrors = true
			log.Printf("[AI][WEB] query=%q error=%v", query, err)
			continue
		}
		log.Printf("[AI][WEB] query=%q snippets=%d", query, len(found))
		for rank, sn := range found {
			key := stationAIWebSnippetDedupKey(sn)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			sn.Score = (len(queries)-i)*100 + (remaining - rank)
			out = append(out, sn)
			if len(out) >= stationAIWebMaxSnippets {
				break
			}
		}
	}

	if len(out) == 0 {
		if hadErrors {
			return nil, []string{unavailableWarn}
		}
		return nil, []string{noDataWarn}
	}
	if hadErrors {
		return out, []string{partialWarn}
	}
	return out, nil
}

func stationAIWebQueryVariants(locale, userMessage string, intent stationAIIntentKind) []string {
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return nil
	}

	out := make([]string, 0, stationAIWebMaxQueries)
	seen := make(map[string]struct{}, stationAIWebMaxQueries*2)
	add := func(query string) {
		query = strings.TrimSpace(query)
		if query == "" {
			return
		}
		norm := strings.Join(strings.Fields(strings.ToLower(query)), " ")
		if norm == "" {
			return
		}
		if _, exists := seen[norm]; exists {
			return
		}
		seen[norm] = struct{}{}
		out = append(out, query)
	}

	add(aiTrimForPrompt(userMessage, 220))

	terms := aiKeywordTerms(userMessage)
	if len(terms) > 0 {
		n := len(terms)
		if n > 8 {
			n = 8
		}
		add(strings.Join(terms[:n], " ") + " eve online")
	}

	switch intent {
	case stationAIIntentTrading:
		add(userMessage + " eve online market trading")
	case stationAIIntentDebug:
		add(userMessage + " EVE Flipper troubleshooting")
	case stationAIIntentProduct:
		add(userMessage + " EVE Flipper wiki docs")
	case stationAIIntentResearch:
		add(userMessage + " latest eve online")
	default:
		add(userMessage + " eve online")
	}

	if locale == "ru" {
		add(userMessage + " EVE Online")
	}

	if len(out) > stationAIWebMaxQueries {
		out = out[:stationAIWebMaxQueries]
	}
	return out
}

func stationAIFetchDuckDuckGoSnippets(
	ctx context.Context,
	client *http.Client,
	query string,
	limit int,
) ([]aiKnowledgeSnippet, error) {
	query = strings.TrimSpace(query)
	if query == "" || limit <= 0 {
		return nil, nil
	}
	reqURL := "https://api.duckduckgo.com/?format=json&no_html=1&skip_disambig=1&q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("duckduckgo http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Heading       string `json:"Heading"`
		AbstractText  string `json:"AbstractText"`
		AbstractURL   string `json:"AbstractURL"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
			Topics   []struct {
				Text     string `json:"Text"`
				FirstURL string `json:"FirstURL"`
			} `json:"Topics"`
		} `json:"RelatedTopics"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	out := make([]aiKnowledgeSnippet, 0, limit)
	push := func(title, link, text string) {
		if len(out) >= limit {
			return
		}
		title = strings.TrimSpace(title)
		link = strings.TrimSpace(link)
		text = aiTrimForPrompt(strings.TrimSpace(text), 500)
		if title == "" || text == "" {
			return
		}
		out = append(out, aiKnowledgeSnippet{
			SourceLabel: "WEB",
			Title:       title,
			URL:         link,
			Content:     text,
			Score:       1,
		})
	}

	if strings.TrimSpace(payload.AbstractText) != "" {
		title := strings.TrimSpace(payload.Heading)
		if title == "" {
			title = "DuckDuckGo"
		}
		push(title, payload.AbstractURL, payload.AbstractText)
	}
	for _, rt := range payload.RelatedTopics {
		if strings.TrimSpace(rt.Text) != "" {
			push("DuckDuckGo related", rt.FirstURL, rt.Text)
			continue
		}
		for _, sub := range rt.Topics {
			push("DuckDuckGo related", sub.FirstURL, sub.Text)
			if len(out) >= limit {
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func stationAIWebSnippetDedupKey(sn aiKnowledgeSnippet) string {
	urlKey := strings.TrimSpace(strings.ToLower(sn.URL))
	if urlKey != "" {
		return "url:" + urlKey
	}
	titleKey := strings.TrimSpace(strings.ToLower(sn.Title))
	contentKey := strings.TrimSpace(strings.ToLower(aiTrimForPrompt(sn.Content, 120)))
	return "txt:" + titleKey + "|" + contentKey
}

func buildStationAIKnowledgeBlock(locale string, wikiSnippets, webSnippets []aiKnowledgeSnippet) string {
	if len(wikiSnippets) == 0 && len(webSnippets) == 0 {
		return ""
	}
	var b strings.Builder
	if locale == "ru" {
		b.WriteString("Structured knowledge context (wiki/web). Используй только если релевантно. Ссылайся на источники строго как [WIKI N] / [WEB N].\n\n")
	} else {
		b.WriteString("Structured knowledge context (wiki/web). Use only when relevant and cite sources strictly as [WIKI N] / [WEB N].\n\n")
	}
	for i, sn := range wikiSnippets {
		page := strings.TrimSpace(sn.Page)
		if page == "" {
			page = sn.Title
		}
		section := strings.TrimSpace(sn.Section)
		if section == "" {
			section = "Overview"
		}
		fmt.Fprintf(
			&b,
			"[WIKI %d]\npage: %s\nsection: %s\nurl: %s\ncontent:\n%s\n\n",
			i+1,
			page,
			section,
			sn.URL,
			sn.Content,
		)
	}
	for i, sn := range webSnippets {
		fmt.Fprintf(
			&b,
			"[WEB %d]\ntitle: %s\nurl: %s\ncontent:\n%s\n\n",
			i+1,
			sn.Title,
			sn.URL,
			sn.Content,
		)
	}
	return b.String()
}

func buildStationAIAgentBlock(locale string, plan stationAIPlannerPlan, ctx stationAIContextPayload, wikiSnippets, webSnippets []aiKnowledgeSnippet) string {
	if len(plan.Agents) == 0 {
		return ""
	}
	lines := make([]string, 0, len(plan.Agents))
	actionableRows := make([]stationAIContextRow, 0, len(ctx.Rows))
	for _, row := range ctx.Rows {
		if strings.EqualFold(strings.TrimSpace(row.Action), "hold") {
			continue
		}
		actionableRows = append(actionableRows, row)
	}
	if len(actionableRows) == 0 {
		actionableRows = append(actionableRows, ctx.Rows...)
	}
	sort.SliceStable(actionableRows, func(i, j int) bool {
		if actionableRows[i].DailyProfit == actionableRows[j].DailyProfit {
			return actionableRows[i].CTS > actionableRows[j].CTS
		}
		return actionableRows[i].DailyProfit > actionableRows[j].DailyProfit
	})
	highRisk := 0
	extreme := 0
	for _, row := range ctx.Rows {
		if row.HighRisk {
			highRisk++
		}
		if row.ExtremePrice {
			extreme++
		}
	}
	if ctx.Runtime != nil {
		if ctx.Runtime.Available {
			riskLabel := "n/a"
			if ctx.Runtime.Risk != nil {
				riskLabel = ctx.Runtime.Risk.RiskLevel
			}
			lines = append(lines, fmt.Sprintf(
				"account_runtime: wallet=%.0f ISK, active_orders=%d (buy=%d sell=%d), tx_%dd=%d, net_flow=%.0f ISK, risk=%s",
				ctx.Runtime.WalletISK,
				ctx.Runtime.ActiveOrders,
				ctx.Runtime.BuyOrders,
				ctx.Runtime.SellOrders,
				ctx.Runtime.TxnWindowDays,
				ctx.Runtime.TransactionCount,
				ctx.Runtime.NetFlowISK,
				riskLabel,
			))
		} else if len(ctx.Runtime.Notes) > 0 {
			lines = append(lines, "account_runtime: unavailable ("+ctx.Runtime.Notes[0]+")")
		}
	}

	for _, agent := range plan.Agents {
		switch agent {
		case "scan_analyzer":
			if len(actionableRows) == 0 {
				continue
			}
			limit := len(actionableRows)
			if limit > 3 {
				limit = 3
			}
			parts := make([]string, 0, limit)
			for i := 0; i < limit; i++ {
				row := actionableRows[i]
				parts = append(parts, fmt.Sprintf("%s(%s, %.0f ISK/day, %.1f%%)", row.TypeName, row.Action, row.DailyProfit, row.Margin))
			}
			lines = append(lines, "scan_analyzer: "+strings.Join(parts, "; "))
		case "risk_checker":
			lines = append(lines, fmt.Sprintf("risk_checker: high_risk=%d, extreme_price=%d, visible_rows=%d", highRisk, extreme, ctx.Summary.VisibleRows))
		case "wiki_retriever":
			if len(wikiSnippets) > 0 {
				lines = append(lines, fmt.Sprintf("wiki_retriever: %d wiki snippets attached", len(wikiSnippets)))
			}
		case "web_retriever":
			if len(webSnippets) > 0 {
				lines = append(lines, fmt.Sprintf("web_retriever: %d web snippets attached", len(webSnippets)))
			}
		case "debug_helper":
			lines = append(lines, fmt.Sprintf("debug_helper: tab=%s, system=%s, scope=%s", ctx.TabID, ctx.SystemName, ctx.StationScope))
		}
	}

	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	if locale == "ru" {
		b.WriteString("Внутренние заметки агентного пайплайна (planner -> executor):\n")
	} else {
		b.WriteString("Internal agent pipeline notes (planner -> executor):\n")
	}
	for _, line := range lines {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func (s *Server) handleAuthStationAIChatStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	writeMsg := func(msg map[string]interface{}) bool {
		line, err := json.Marshal(msg)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	writeErr := func(message string) {
		_ = writeMsg(map[string]interface{}{
			"type":    "error",
			"message": message,
		})
	}

	var req stationAIChatRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr("invalid json")
		return
	}

	enableWiki, enableWeb, warnings, validationErr := normalizeStationAIChatRequest(&req)
	if validationErr != "" {
		writeErr(validationErr)
		return
	}
	plan, plannerEnabled, plannerWarnings := s.stationAIResolvePlan(r.Context(), req)
	warnings = append(warnings, plannerWarnings...)
	intent := plan.Intent
	useWiki := enableWiki && intent != stationAIIntentSmallTalk
	useWeb := enableWeb && intent != stationAIIntentSmallTalk
	useRuntime := stationAINeedsRuntimeContext(intent, req.UserMessage)
	pipeline := stationAIPipelineMeta(req, plan, plannerEnabled)
	log.Printf(
		"[AI][CHAT] mode=stream intent=%s planner=%t wiki_enabled=%t need_wiki=%t wiki_used=%t web_enabled=%t need_web=%t web_used=%t runtime_requested=%t locale=%s model=%s",
		intent,
		plannerEnabled,
		enableWiki,
		plan.NeedWiki,
		useWiki,
		enableWeb,
		plan.NeedWeb,
		useWeb,
		useRuntime,
		req.Locale,
		req.Model,
	)
	if len(plannerWarnings) > 0 {
		log.Printf("[AI][CHAT] mode=stream planner_warnings=%v", plannerWarnings)
	}

	prepareMsg := "Preparing context..."
	plannerMsg := "Planner pass complete"
	sendMsg := "Sending request to OpenRouter..."
	streamMsg := "Streaming model output..."
	retryMsg := "Final answer failed validation, retrying..."
	doneMsg := "Done"
	if req.Locale == "ru" {
		prepareMsg = "Подготавливаю контекст..."
		plannerMsg = "Планировщик определил режим ответа"
		sendMsg = "Отправляю запрос в OpenRouter..."
		streamMsg = "Получаю ответ модели..."
		retryMsg = "Финальный ответ не прошел валидацию, выполняю retry..."
		doneMsg = "Готово"
	}
	if !writeMsg(map[string]interface{}{
		"type":         "progress",
		"message":      prepareMsg,
		"progress_pct": 8,
	}) {
		return
	}

	if plannerEnabled {
		if !writeMsg(map[string]interface{}{
			"type":         "progress",
			"message":      plannerMsg,
			"progress_pct": 14,
		}) {
			return
		}
	}
	if plan.AskClarification && strings.TrimSpace(plan.Clarification) != "" {
		warnings = append(warnings, "planner asked for clarification")
		_ = writeMsg(map[string]interface{}{
			"type":          "result",
			"answer":        strings.TrimSpace(plan.Clarification),
			"provider":      req.Provider,
			"model":         req.Model,
			"assistant":     req.AssistantName,
			"intent":        string(intent),
			"pipeline":      pipeline,
			"warnings":      warnings,
			"progress_pct":  100,
			"progress_text": doneMsg,
		})
		return
	}

	contextForPrompt := stationAIContextForPlan(req.Context, plan)
	runtimeUsed := false
	if useRuntime {
		userID := userIDFromRequest(r)
		runtimeCtx, rw := s.stationAIBuildRuntimeContext(r.Context(), userID, req.Locale)
		if runtimeCtx != nil {
			contextForPrompt.Runtime = runtimeCtx
			runtimeUsed = runtimeCtx.Available
		}
		warnings = append(warnings, rw...)
		log.Printf("[AI][CHAT] mode=stream runtime_requested=%t runtime_used=%t", useRuntime, runtimeUsed)
	}
	preflight := stationAIPreflight(req.Locale, plan, contextForPrompt, useRuntime)
	pipeline["preflight_status"] = preflight.Status
	if len(preflight.Missing) > 0 {
		pipeline["preflight_missing"] = preflight.Missing
	}
	if len(preflight.Caveats) > 0 {
		pipeline["preflight_caveats"] = preflight.Caveats
	}
	if preflight.Status == "fail" {
		warnings = append(warnings, "preflight failed: missing "+strings.Join(preflight.Missing, ", "))
		_ = writeMsg(map[string]interface{}{
			"type":          "result",
			"answer":        stationAIPreflightFailAnswer(req.Locale, preflight.Missing),
			"provider":      req.Provider,
			"model":         req.Model,
			"assistant":     req.AssistantName,
			"intent":        string(intent),
			"pipeline":      pipeline,
			"warnings":      warnings,
			"progress_pct":  100,
			"progress_text": doneMsg,
		})
		log.Printf("[AI][CHAT] mode=stream preflight=fail intent=%s missing=%v", intent, preflight.Missing)
		return
	}
	contextJSON, err := json.Marshal(contextForPrompt)
	if err != nil {
		writeErr("failed to encode context")
		return
	}
	wikiSnippets := make([]aiKnowledgeSnippet, 0, 4)
	webSnippets := make([]aiKnowledgeSnippet, 0, 4)
	if useWiki {
		ws, ww := s.stationAIWikiSnippets(r.Context(), req.Locale, req.UserMessage, req.WikiRepo, intent)
		wikiSnippets = ws
		warnings = append(warnings, ww...)
	} else if enableWiki {
		if intent == stationAIIntentSmallTalk {
			log.Printf("[AI][CHAT] mode=stream wiki skipped for smalltalk intent")
		} else if !plan.NeedWiki {
			log.Printf("[AI][CHAT] mode=stream wiki skipped by planner intent=%s", intent)
		}
	}
	if useWeb {
		ws, ww := stationAIWebSnippets(r.Context(), req.Locale, req.UserMessage, intent)
		webSnippets = ws
		warnings = append(warnings, ww...)
	} else if enableWeb {
		log.Printf("[AI][CHAT] mode=stream web skipped for smalltalk intent")
	}
	knowledgeBlock := buildStationAIKnowledgeBlock(req.Locale, wikiSnippets, webSnippets)
	agentBlock := buildStationAIAgentBlock(req.Locale, plan, contextForPrompt, wikiSnippets, webSnippets)

	systemPrompt := stationAISystemPrompt(req.Locale, req.AssistantName, plan)
	userPrompt := stationAIUserPrompt(req.Locale, req.UserMessage, contextJSON, plan)
	if caveatBlock := stationAIPreflightCaveatBlock(req.Locale, preflight); caveatBlock != "" {
		userPrompt += "\n\n" + caveatBlock
	}
	if agentBlock != "" {
		userPrompt += "\n\n" + agentBlock
	}
	if knowledgeBlock != "" {
		userPrompt += "\n\n" + knowledgeBlock
	}
	messages := buildStationAIMessages(systemPrompt, req.History, userPrompt)
	promptTokensEst := estimateTokensFromText(systemPrompt) + estimateTokensFromText(userPrompt)
	for _, msg := range req.History {
		promptTokensEst += estimateTokensFromText(msg.Content)
	}

	if !writeMsg(map[string]interface{}{
		"type":              "progress",
		"message":           sendMsg,
		"progress_pct":      20,
		"prompt_tokens_est": promptTokensEst,
		"total_tokens_est":  promptTokensEst,
	}) {
		return
	}

	payload := map[string]interface{}{
		"model":       req.Model,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"stream":      true,
		"stream_options": map[string]bool{
			"include_usage": true,
		},
		"messages": messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		writeErr("failed to encode ai request")
		return
	}

	httpReq, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodPost,
		"https://openrouter.ai/api/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		writeErr("failed to create ai request")
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("HTTP-Referer", "http://localhost:1420")
	httpReq.Header.Set("X-Title", "EVE Flipper Station AI")

	client := &http.Client{Timeout: stationAIStreamHTTPTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		writeErr("ai provider request failed: " + err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawResp, readErr := readBodyWithLimit(resp.Body, stationAIProviderErrorMaxBodyBytes)
		if readErr != nil {
			writeErr("ai provider error")
			return
		}
		errMsg := "ai provider error"
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(rawResp, &errBody) == nil && strings.TrimSpace(errBody.Error.Message) != "" {
			errMsg = strings.TrimSpace(errBody.Error.Message)
		}
		writeErr(errMsg)
		return
	}

	if !writeMsg(map[string]interface{}{
		"type":         "progress",
		"message":      streamMsg,
		"progress_pct": 35,
	}) {
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 128*1024), 8*1024*1024)
	var eventData strings.Builder
	var answerBuilder strings.Builder
	answerRuneCount := 0
	providerModel := req.Model
	providerID := ""
	usagePrompt := 0
	usageCompletion := 0
	usageTotal := 0

	processEvent := func(raw string) (done bool, fatal bool) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return false, false
		}
		if raw == "[DONE]" {
			return true, false
		}

		var chunk struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content json.RawMessage `json:"content"`
				} `json:"delta"`
				Message struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
			// Ignore malformed partial event and keep stream alive.
			return false, false
		}

		if strings.TrimSpace(chunk.ID) != "" {
			providerID = strings.TrimSpace(chunk.ID)
		}
		if strings.TrimSpace(chunk.Model) != "" {
			providerModel = strings.TrimSpace(chunk.Model)
		}

		for _, choice := range chunk.Choices {
			delta := extractAIDelta(choice.Delta.Content)
			if delta == "" {
				delta = extractAIDelta(choice.Message.Content)
			}
			if delta == "" {
				continue
			}
			answerBuilder.WriteString(delta)
			answerRuneCount += utf8.RuneCountInString(delta)

			completionTokensEst := estimateTokensFromRuneCount(answerRuneCount)
			totalTokensEst := promptTokensEst + completionTokensEst
			denom := req.MaxTokens
			if denom < 500 {
				denom = 500
			}
			if denom > 6000 {
				denom = 6000
			}
			progressPct := 45 + int(float64(completionTokensEst)*45.0/float64(denom))
			if progressPct > 90 {
				progressPct = 90
			}

			if !writeMsg(map[string]interface{}{
				"type":                  "delta",
				"delta":                 delta,
				"progress_pct":          progressPct,
				"completion_tokens_est": completionTokensEst,
				"total_tokens_est":      totalTokensEst,
			}) {
				return false, true
			}
		}

		if chunk.Usage != nil && chunk.Usage.TotalTokens > 0 {
			usagePrompt = chunk.Usage.PromptTokens
			usageCompletion = chunk.Usage.CompletionTokens
			usageTotal = chunk.Usage.TotalTokens
			if !writeMsg(map[string]interface{}{
				"type":              "usage",
				"prompt_tokens":     usagePrompt,
				"completion_tokens": usageCompletion,
				"total_tokens":      usageTotal,
				"progress_pct":      96,
			}) {
				return false, true
			}
		}

		return false, false
	}

	stop := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			done, fatal := processEvent(eventData.String())
			eventData.Reset()
			if fatal {
				return
			}
			if done {
				stop = true
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if eventData.Len() > 0 {
				eventData.WriteByte('\n')
			}
			eventData.WriteString(data)
		}
	}
	if !stop && eventData.Len() > 0 {
		_, fatal := processEvent(eventData.String())
		if fatal {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("[AI][CHAT] mode=stream read_error=%v", err)
		writeErr("failed to read ai stream: " + err.Error())
		return
	}

	answer := strings.TrimSpace(answerBuilder.String())
	if answer == "" {
		writeErr("empty ai answer")
		return
	}
	if valid, issue := stationAIValidateAnswer(answer, intent); !valid {
		warnings = append(warnings, "server validation requested retry: "+issue)
		_ = writeMsg(map[string]interface{}{
			"type":         "progress",
			"message":      retryMsg,
			"progress_pct": 94,
		})
		retryMessages := make([]map[string]string, 0, len(messages)+2)
		retryMessages = append(retryMessages, messages...)
		retryMessages = append(retryMessages,
			map[string]string{"role": "assistant", "content": answer},
			map[string]string{"role": "user", "content": stationAIRetryCorrectionPrompt(req.Locale, issue)},
		)
		retryReply, retryErr := s.stationAIOpenRouterChatOnce(r.Context(), req, retryMessages)
		if retryErr != nil {
			warnings = append(warnings, "retry failed: "+retryErr.Error())
		} else if validRetry, retryIssue := stationAIValidateAnswer(retryReply.Answer, intent); validRetry {
			answer = retryReply.Answer
			if strings.TrimSpace(retryReply.Model) != "" {
				providerModel = retryReply.Model
			}
			if strings.TrimSpace(retryReply.ProviderID) != "" {
				providerID = retryReply.ProviderID
			}
			if p, c, t := stationAIUsageTokenInts(retryReply.Usage); t > 0 {
				usagePrompt = p
				usageCompletion = c
				usageTotal = t
			}
		} else {
			warnings = append(warnings, "retry rejected: "+retryIssue)
		}
	}

	result := map[string]interface{}{
		"type":          "result",
		"answer":        answer,
		"provider":      req.Provider,
		"model":         providerModel,
		"assistant":     req.AssistantName,
		"intent":        string(intent),
		"pipeline":      pipeline,
		"warnings":      warnings,
		"provider_id":   providerID,
		"progress_pct":  100,
		"progress_text": doneMsg,
	}
	if usageTotal > 0 {
		result["usage"] = map[string]int{
			"prompt_tokens":     usagePrompt,
			"completion_tokens": usageCompletion,
			"total_tokens":      usageTotal,
		}
	}

	_ = writeMsg(result)
	log.Printf(
		"[AI][CHAT] mode=stream done intent=%s wiki_snippets=%d web_snippets=%d warnings=%d provider_model=%s",
		intent,
		len(wikiSnippets),
		len(webSnippets),
		len(warnings),
		providerModel,
	)
}

func stationAIIntentPolicy(locale string, plan stationAIPlannerPlan) string {
	var intentPolicy string
	if locale == "ru" {
		switch plan.Intent {
		case stationAIIntentSmallTalk:
			intentPolicy = "Режим smalltalk: ответь коротко (1-3 предложения), естественно и по теме. Не давай торговые рекомендации, пока пользователь прямо не запросит анализ/действия."
		case stationAIIntentTrading:
			intentPolicy = "Режим trading_analysis: дай прикладной ответ по скану. Формат: 1) Рекомендация 2) Почему 3) Риски 4) Следующие действия/фильтры. Опирайся только на данные контекста."
		case stationAIIntentProduct:
			intentPolicy = "Режим product_help: объясняй функциональность проекта и workflow. Если используешь wiki/web контекст, добавляй ссылки в формате [WIKI N]/[WEB N]."
		case stationAIIntentDebug:
			intentPolicy = "Режим debug_support: сначала краткий диагноз, затем проверочные шаги и предложенный фикс. Не придумывай факты, явно указывай неопределенность."
		case stationAIIntentResearch:
			intentPolicy = "Режим web_research: суммируй только подтвержденные внешние факты, добавляй источники [WEB N], отделяй факты от предположений."
		default:
			intentPolicy = "Режим general: ответь по запросу пользователя без навязанных торговых рекомендаций. Если пользователь захочет анализ скана, предложи перейти к нему."
		}
		switch plan.ResponseMode {
		case "short":
			intentPolicy += " Ответ должен быть кратким."
		case "structured":
			intentPolicy += " Сохраняй структурированный формат и конкретные шаги."
		case "diagnostic":
			intentPolicy += " Делай акцент на проверках, гипотезах и воспроизводимости."
		}
		return intentPolicy
	}

	switch plan.Intent {
	case stationAIIntentSmallTalk:
		intentPolicy = "smalltalk mode: reply naturally in 1-3 short sentences. Do not provide trading recommendations unless explicitly asked for scan/trade analysis."
	case stationAIIntentTrading:
		intentPolicy = "trading_analysis mode: provide actionable scan guidance. Format: 1) Recommendation 2) Why 3) Risk check 4) Next actions/filters. Use only supplied data. If user explicitly asks for a full list of scan settings/parameters, prioritize exhaustive scan_snapshot field enumeration (standard + advanced) over the short recommendation template."
	case stationAIIntentProduct:
		intentPolicy = "product_help mode: explain product features/workflow. When using wiki/web context, cite sources as [WIKI N]/[WEB N]."
	case stationAIIntentDebug:
		intentPolicy = "debug_support mode: provide concise diagnosis first, then checks and fix steps. Never invent facts; call out uncertainty explicitly."
	case stationAIIntentResearch:
		intentPolicy = "web_research mode: summarize only grounded external findings, cite [WEB N], separate facts from inferences."
	default:
		intentPolicy = "general mode: answer user intent directly without unsolicited trade advice. Offer scan analysis only when user asks for it."
	}
	switch plan.ResponseMode {
	case "short":
		intentPolicy += " Keep the answer compact."
	case "structured":
		intentPolicy += " Keep the answer structured with explicit steps."
	case "diagnostic":
		intentPolicy += " Focus on diagnosis, checks, and reproducible fixes."
	}
	return intentPolicy
}

func stationAIRequestsFullScanSettings(userMessage string) bool {
	msg := strings.TrimSpace(strings.ToLower(userMessage))
	if msg == "" {
		return false
	}

	fullTerms := []string{
		"full list", "complete list", "all fields", "every field", "all parameters", "full snapshot",
		"полный список", "полный перечень", "все поля", "все параметры", "весь список",
	}
	settingsTerms := []string{
		"scan settings", "scan parameters", "scan params", "settings", "parameters", "params", "scan snapshot",
		"настрой", "параметр", "поля", "адванс", "advanced", "обычн", "стандарт",
	}
	explicitPhrases := []string{
		"list all scan parameters",
		"show all scan settings",
		"give me full scan settings",
		"перечисли все настройки скан",
		"перечисли все параметры скан",
		"дай полный список настроек скан",
		"полный список настроек скан",
	}

	if containsAnyLower(msg, explicitPhrases) {
		return true
	}
	return containsAnyLower(msg, fullTerms) && containsAnyLower(msg, settingsTerms)
}

func stationAISystemPrompt(locale, assistantName string, plan stationAIPlannerPlan) string {
	policy := stationAIIntentPolicy(locale, plan)
	agents := "none"
	if len(plan.Agents) > 0 {
		agents = strings.Join(plan.Agents, ", ")
	}
	if locale == "ru" {
		return fmt.Sprintf(
			"Ты %s, AI-ассистент проекта EVE Flipper. "+
				"Ты работаешь во втором шаге пайплайна после planner-а. "+
				"Используй только предоставленные данные (контекст таба/скана, runtime-контекст аккаунта, заметки агентного пайплайна, проектная документация, wiki/web snippets). "+
				"Разделяй типы данных: runtime-данные таба/скана и runtime-контекст аккаунта — для чисел и текущего состояния, wiki/web — только для описания механик продукта. "+
				"Если утверждение основано на wiki/web, обязательно ссылайся на конкретный источник [WIKI N]/[WEB N]. "+
				"Перед тем как писать, что 'в документации нет информации', проверь все переданные wiki-snippets: если релевантный факт уже есть в них, используй его и не заявляй о пробеле. "+
				"Не выдумывай цены, объемы, ID и внешние факты. Если данных недостаточно — скажи прямо. "+
				"Подстраивайся под тон пользователя и поддерживай язык запроса. "+
				"Активные агенты planner-а: %s. "+
				"%s",
			assistantName,
			agents,
			policy,
		)
	}
	return fmt.Sprintf(
		"You are %s, the EVE Flipper project copilot. "+
			"You run as executor step after a planner pass. "+
			"Use only supplied data (tab/scan context, account runtime context, agent pipeline notes, project docs, wiki/web snippets). "+
			"Strictly separate data domains: runtime tab/scan context and account runtime context are for live numbers/state, wiki/web snippets are for product mechanics only. "+
			"If a statement comes from wiki/web, cite the exact source as [WIKI N]/[WEB N]. "+
			"Before claiming 'the documentation does not specify', verify all provided wiki snippets; if a relevant fact is present, use it and do not claim a gap. "+
			"Never invent prices, volumes, IDs, or external facts. If context is insufficient, state that clearly. "+
			"Adapt to the user's tone and language. "+
			"Planner-selected agents: %s. "+
			"%s",
		assistantName,
		agents,
		policy,
	)
}

func stationAIUserPrompt(locale, userMessage string, contextJSON []byte, plan stationAIPlannerPlan) string {
	agents := "none"
	if len(plan.Agents) > 0 {
		agents = strings.Join(plan.Agents, ", ")
	}
	fullSettingsRequest := stationAIRequestsFullScanSettings(userMessage)
	if locale == "ru" {
		extra := ""
		if fullSettingsRequest {
			extra = "\n\nПользователь явно просит полный список параметров сканирования. " +
				"В ответе выведи исчерпывающий инвентарь ВСЕХ полей объекта scan_snapshot из JSON выше " +
				"(обычные + advanced) и их текущие значения. " +
				"Форматируй построчно как field_name: value. Ничего не пропускай, даже если значение 0/false/пусто."
		}
		return fmt.Sprintf(
			"Planner plan:\nintent=%s\ncontext_level=%s\nresponse_mode=%s\nagents=%s\nneed_wiki=%t\nneed_web=%t\n\n"+
				"Вопрос пользователя:\n%s\n\n"+
				"Контекст текущего таба/скана (JSON):\n%s\n\n"+
				"Ответь на русском языке, дай прямой практический ответ строго на основе переданного контекста.%s",
			string(plan.Intent),
			plan.ContextLevel,
			plan.ResponseMode,
			agents,
			plan.NeedWiki,
			plan.NeedWeb,
			userMessage,
			string(contextJSON),
			extra,
		)
	}
	extra := ""
	if fullSettingsRequest {
		extra = "\n\nThe user explicitly asked for a full scan settings list. " +
			"Output a complete inventory of ALL fields from the scan_snapshot object in the JSON above " +
			"(standard + advanced) with current values. " +
			"Use one field per line as field_name: value. Do not omit fields, even when values are 0/false/empty."
	}
	return fmt.Sprintf(
		"Planner plan:\nintent=%s\ncontext_level=%s\nresponse_mode=%s\nagents=%s\nneed_wiki=%t\nneed_web=%t\n\n"+
			"User question:\n%s\n\n"+
			"Current tab/scan context (JSON):\n%s\n\n"+
			"Answer in English with a direct practical response grounded in the provided context.%s",
		string(plan.Intent),
		plan.ContextLevel,
		plan.ResponseMode,
		agents,
		plan.NeedWiki,
		plan.NeedWeb,
		userMessage,
		string(contextJSON),
		extra,
	)
}

func extractAIContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if strings.TrimSpace(p.Text) == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(p.Text)
		}
		return strings.TrimSpace(b.String())
	}
	return strings.TrimSpace(string(raw))
}

func extractAIDelta(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Text == "" {
				continue
			}
			b.WriteString(p.Text)
		}
		return b.String()
	}
	return ""
}

func (s *Server) handleAuthPortfolio(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, true)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}

	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
			days = d
		}
	}
	salesTax := 8.0
	if cfg := s.loadConfigForUser(userID); cfg != nil {
		salesTax = cfg.SalesTaxPercent
	}
	if v := r.URL.Query().Get("sales_tax"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 100 {
			salesTax = f
		}
	}
	brokerFee := 1.0
	if v := r.URL.Query().Get("broker_fee"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 100 {
			brokerFee = f
		}
	}
	ledgerLimit := 500
	if v := r.URL.Query().Get("ledger_limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 5000 {
			ledgerLimit = n
		}
	}

	fetchTxns := func(sess *auth.Session) ([]esi.WalletTransaction, error) {
		if cached, ok := s.getWalletTxnCache(sess.CharacterID); ok {
			return cached, nil
		}
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			return nil, tokenErr
		}
		freshTxns, fetchErr := s.esi.GetWalletTransactions(sess.CharacterID, token)
		if fetchErr != nil {
			return nil, fetchErr
		}

		// Enrich type names from SDE
		s.mu.RLock()
		sdeData := s.sdeData
		s.mu.RUnlock()
		if sdeData != nil {
			for i := range freshTxns {
				if t, ok := sdeData.Types[freshTxns[i].TypeID]; ok {
					freshTxns[i].TypeName = t.Name
				}
			}
		}
		s.setWalletTxnCache(sess.CharacterID, freshTxns)
		return freshTxns, nil
	}

	var txns []esi.WalletTransaction
	for _, sess := range selectedSessions {
		part, fetchErr := fetchTxns(sess)
		if fetchErr != nil {
			log.Printf("[AUTH] Portfolio txns error (%s): %v", sess.CharacterName, fetchErr)
			if !allScope {
				writeError(w, 500, "failed to fetch transactions: "+fetchErr.Error())
				return
			}
			continue
		}
		txns = append(txns, part...)
	}
	if len(txns) == 0 && len(selectedSessions) > 0 {
		if allScope {
			writeError(w, 500, "failed to fetch transactions for selected characters")
		} else {
			writeError(w, 500, "failed to fetch transactions")
		}
		return
	}

	result := engine.ComputePortfolioPnLWithOptions(txns, engine.PortfolioPnLOptions{
		LookbackDays:         days,
		SalesTaxPercent:      salesTax,
		BrokerFeePercent:     brokerFee,
		LedgerLimit:          ledgerLimit,
		IncludeUnmatchedSell: false, // strict realized mode for API
	})
	writeJSON(w, result)
}

func (s *Server) handleAuthPortfolioOptimize(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, true)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}

	daysStr := r.URL.Query().Get("days")
	days := 90
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
			days = d
		}
	}

	fetchTxns := func(sess *auth.Session) ([]esi.WalletTransaction, error) {
		if cached, ok := s.getWalletTxnCache(sess.CharacterID); ok {
			return cached, nil
		}
		token, tokenErr := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if tokenErr != nil {
			return nil, tokenErr
		}
		freshTxns, fetchErr := s.esi.GetWalletTransactions(sess.CharacterID, token)
		if fetchErr != nil {
			return nil, fetchErr
		}

		// Enrich type names from SDE
		s.mu.RLock()
		sdeData := s.sdeData
		s.mu.RUnlock()
		if sdeData != nil {
			for i := range freshTxns {
				if t, ok := sdeData.Types[freshTxns[i].TypeID]; ok {
					freshTxns[i].TypeName = t.Name
				}
			}
		}
		s.setWalletTxnCache(sess.CharacterID, freshTxns)
		return freshTxns, nil
	}

	var txns []esi.WalletTransaction
	for _, sess := range selectedSessions {
		part, fetchErr := fetchTxns(sess)
		if fetchErr != nil {
			log.Printf("[AUTH] Optimizer txns error (%s): %v", sess.CharacterName, fetchErr)
			if !allScope {
				writeError(w, 500, "failed to fetch transactions: "+fetchErr.Error())
				return
			}
			continue
		}
		txns = append(txns, part...)
	}
	if len(txns) == 0 && len(selectedSessions) > 0 {
		if allScope {
			writeError(w, 500, "failed to fetch transactions for selected characters")
		} else {
			writeError(w, 500, "failed to fetch transactions")
		}
		return
	}

	result, diag := engine.ComputePortfolioOptimization(txns, days)
	if result == nil {
		// Return diagnostic info as JSON so the frontend can show details.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{
			"error":      "not enough trading data for optimization",
			"diagnostic": diag,
		})
		return
	}

	writeJSON(w, result)
}

// --- Industry Handlers ---

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampInt32(value, minValue, maxValue int32) int32 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampFloat64(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func (s *Server) handleIndustryAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TypeID             int32   `json:"type_id"`
		Runs               int32   `json:"runs"`
		MaterialEfficiency int32   `json:"me"`
		TimeEfficiency     int32   `json:"te"`
		SystemName         string  `json:"system_name"`
		StationID          int64   `json:"station_id"` // Optional: specific station/structure for price lookup
		FacilityTax        float64 `json:"facility_tax"`
		StructureBonus     float64 `json:"structure_bonus"`
		BrokerFee          float64 `json:"broker_fee"`
		SalesTaxPercent    float64 `json:"sales_tax_percent"`
		MaxDepth           int     `json:"max_depth"`
		OwnBlueprint       *bool   `json:"own_blueprint"` // nil → true (default)
		BlueprintCost      float64 `json:"blueprint_cost"`
		BlueprintIsBPO     bool    `json:"blueprint_is_bpo"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, industryAnalyzeMaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, 400, "invalid json")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, 400, "invalid json")
		return
	}

	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	if req.TypeID <= 0 {
		writeError(w, 400, "type_id is required")
		return
	}
	req.Runs = clampInt32(req.Runs, 1, industryAnalyzeMaxRuns)
	req.MaterialEfficiency = clampInt32(req.MaterialEfficiency, 0, 10)
	req.TimeEfficiency = clampInt32(req.TimeEfficiency, 0, 20)
	req.MaxDepth = clampInt(req.MaxDepth, 1, industryAnalyzeMaxDepth)
	req.FacilityTax = clampFloat64(req.FacilityTax, 0, 100)
	req.StructureBonus = clampFloat64(req.StructureBonus, -100, 100)
	req.BrokerFee = clampFloat64(req.BrokerFee, 0, 100)
	req.SalesTaxPercent = clampFloat64(req.SalesTaxPercent, 0, 100)
	if req.StationID < 0 {
		req.StationID = 0
	}
	if req.BlueprintCost < 0 {
		req.BlueprintCost = 0
	}
	req.SystemName = strings.TrimSpace(req.SystemName)

	// Resolve system ID
	var systemID int32
	if req.SystemName != "" {
		s.mu.RLock()
		systemID = s.sdeData.SystemByName[strings.ToLower(req.SystemName)]
		s.mu.RUnlock()
	}

	params := engine.IndustryParams{
		TypeID:             req.TypeID,
		Runs:               req.Runs,
		MaterialEfficiency: req.MaterialEfficiency,
		TimeEfficiency:     req.TimeEfficiency,
		SystemID:           systemID,
		StationID:          req.StationID,
		FacilityTax:        req.FacilityTax,
		StructureBonus:     req.StructureBonus,
		BrokerFee:          req.BrokerFee,
		SalesTaxPercent:    req.SalesTaxPercent,
		MaxDepth:           req.MaxDepth,
		OwnBlueprint:       req.OwnBlueprint == nil || *req.OwnBlueprint,
		BlueprintCost:      req.BlueprintCost,
		BlueprintIsBPO:     req.BlueprintIsBPO,
	}

	// Use NDJSON streaming for progress
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	s.mu.RLock()
	analyzer := s.industryAnalyzer
	s.mu.RUnlock()

	log.Printf("[API] IndustryAnalyze: typeID=%d, runs=%d, ME=%d, TE=%d, system=%s",
		req.TypeID, req.Runs, req.MaterialEfficiency, req.TimeEfficiency, req.SystemName)

	startTime := time.Now()

	result, err := analyzer.Analyze(params, func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	})

	if err != nil {
		log.Printf("[API] IndustryAnalyze error: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("[API] IndustryAnalyze complete in %dms", durationMs)

	line, _ := json.Marshal(map[string]interface{}{"type": "result", "data": result})
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

func (s *Server) handleIndustrySearch(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 128 {
		query = query[:128]
	}
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	limit = clampInt(limit, 1, industrySearchMaxLimit)

	s.mu.RLock()
	analyzer := s.industryAnalyzer
	s.mu.RUnlock()

	if analyzer == nil {
		log.Printf("[API] IndustrySearch: analyzer is nil!")
		writeJSON(w, []struct{}{})
		return
	}

	results := analyzer.SearchBuildableItems(query, limit)
	log.Printf("[API] IndustrySearch: query=%q, found %d results", query, len(results))
	writeJSON(w, results)
}

func (s *Server) handleIndustrySystems(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	// Return list of systems with cost indices
	systems, err := s.esi.FetchIndustrySystems()
	if err != nil {
		writeError(w, 500, "failed to fetch industry systems: "+err.Error())
		return
	}

	// Enrich with system names
	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	type SystemWithName struct {
		SolarSystemID   int32   `json:"solar_system_id"`
		SolarSystemName string  `json:"solar_system_name"`
		Manufacturing   float64 `json:"manufacturing"`
		Reaction        float64 `json:"reaction"`
		Copying         float64 `json:"copying"`
		Invention       float64 `json:"invention"`
	}

	result := make([]SystemWithName, 0, len(systems))
	for _, sys := range systems {
		name := ""
		if s, ok := sdeData.Systems[sys.SolarSystemID]; ok {
			name = s.Name
		}

		swn := SystemWithName{
			SolarSystemID:   sys.SolarSystemID,
			SolarSystemName: name,
		}

		for _, ci := range sys.CostIndices {
			switch ci.Activity {
			case "manufacturing":
				swn.Manufacturing = ci.CostIndex
			case "reaction":
				swn.Reaction = ci.CostIndex
			case "copying":
				swn.Copying = ci.CostIndex
			case "invention":
				swn.Invention = ci.CostIndex
			}
		}

		result = append(result, swn)
	}

	writeJSON(w, result)
}

func (s *Server) handleIndustryStatus(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	s.mu.RLock()
	sdeData := s.sdeData
	s.mu.RUnlock()

	blueprintCount := 0
	productCount := 0
	if sdeData.Industry != nil {
		blueprintCount = len(sdeData.Industry.Blueprints)
		productCount = len(sdeData.Industry.ProductToBlueprint)
	}

	writeJSON(w, map[string]interface{}{
		"blueprints_loaded":   blueprintCount,
		"products_with_bp":    productCount,
		"total_types":         len(sdeData.Types),
		"industry_data_ready": sdeData.Industry != nil,
	})
}

// --- Demand / War Tracker Handlers ---

// handleDemandRegions returns cached demand data for all regions.
func (s *Server) handleDemandRegions(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	// Try to get from cache first
	regions, err := s.db.GetDemandRegions()
	if err != nil {
		writeError(w, 500, "failed to get demand regions: "+err.Error())
		return
	}

	// If cache is empty or stale, return what we have but suggest refresh
	cacheAge := 0
	if len(regions) > 0 {
		cacheAge = int(time.Since(regions[0].UpdatedAt).Minutes())
	}

	writeJSON(w, map[string]interface{}{
		"regions":           regions,
		"count":             len(regions),
		"cache_age_minutes": cacheAge,
		"stale":             len(regions) == 0 || !s.db.IsDemandCacheFresh(60*time.Minute),
	})
}

// handleDemandHotZones returns regions with elevated kill activity.
func (s *Server) handleDemandHotZones(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Check if cache is fresh (less than 1 hour old)
	if s.db.IsDemandCacheFresh(60 * time.Minute) {
		// Return from cache
		zones, err := s.db.GetHotZones(limit)
		if err != nil {
			writeError(w, 500, "failed to get hot zones: "+err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{
			"hot_zones":  zones,
			"count":      len(zones),
			"from_cache": true,
		})
		return
	}

	// Cache is stale - fetch fresh data
	s.mu.RLock()
	analyzer := s.demandAnalyzer
	s.mu.RUnlock()

	if analyzer == nil {
		writeError(w, 503, "demand analyzer not ready")
		return
	}

	zones, err := analyzer.GetHotZones(limit)
	if err != nil {
		writeError(w, 500, "failed to analyze hot zones: "+err.Error())
		return
	}

	// Cache the results
	for _, z := range zones {
		s.db.SaveDemandRegion(&db.DemandRegion{
			RegionID:      z.RegionID,
			RegionName:    z.RegionName,
			HotScore:      z.HotScore,
			Status:        z.Status,
			KillsToday:    z.KillsToday,
			KillsBaseline: z.KillsBaseline,
			ISKDestroyed:  z.ISKDestroyed,
			ActivePlayers: z.ActivePlayers,
			TopShips:      z.TopShips,
		})
	}

	writeJSON(w, map[string]interface{}{
		"hot_zones":  zones,
		"count":      len(zones),
		"from_cache": false,
	})
}

// handleDemandRegion returns detailed demand data for a single region.
func (s *Server) handleDemandRegion(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	regionIDStr := r.PathValue("regionID")
	regionID, err := strconv.ParseInt(regionIDStr, 10, 32)
	if err != nil {
		writeError(w, 400, "invalid region ID")
		return
	}

	// Try cache first
	cached, err := s.db.GetDemandRegion(int32(regionID))
	if err != nil {
		writeError(w, 500, "failed to get region: "+err.Error())
		return
	}

	if cached != nil && time.Since(cached.UpdatedAt) < 60*time.Minute {
		writeJSON(w, map[string]interface{}{
			"region":     cached,
			"from_cache": true,
		})
		return
	}

	// Fetch fresh data
	s.mu.RLock()
	analyzer := s.demandAnalyzer
	s.mu.RUnlock()

	if analyzer == nil {
		writeError(w, 503, "demand analyzer not ready")
		return
	}

	zone, err := analyzer.GetSingleRegionStats(int32(regionID))
	if err != nil {
		writeError(w, 500, "failed to get region stats: "+err.Error())
		return
	}

	if zone == nil {
		writeError(w, 404, "region not found")
		return
	}

	// Cache the result
	s.db.SaveDemandRegion(&db.DemandRegion{
		RegionID:      zone.RegionID,
		RegionName:    zone.RegionName,
		HotScore:      zone.HotScore,
		Status:        zone.Status,
		KillsToday:    zone.KillsToday,
		KillsBaseline: zone.KillsBaseline,
		ISKDestroyed:  zone.ISKDestroyed,
		ActivePlayers: zone.ActivePlayers,
		TopShips:      zone.TopShips,
	})

	writeJSON(w, map[string]interface{}{
		"region":     zone,
		"from_cache": false,
	})
}

// handleDemandOpportunities returns trade opportunities for a specific region.
func (s *Server) handleDemandOpportunities(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	regionIDStr := r.PathValue("regionID")
	regionIDInt, err := strconv.Atoi(regionIDStr)
	if err != nil {
		writeError(w, 400, "invalid region ID")
		return
	}
	regionID := int32(regionIDInt)

	s.mu.RLock()
	analyzer := s.demandAnalyzer
	esiClient := s.esi
	sdeData := s.sdeData
	s.mu.RUnlock()

	if analyzer == nil {
		writeError(w, 503, "demand analyzer not ready")
		return
	}

	// Try to load fitting profile from cache (TTL 2 hours)
	var fittingProfile *zkillboard.RegionDemandProfile
	if s.db.IsFittingProfileFresh(regionID, 2*time.Hour) {
		items, err := s.db.GetFittingDemandProfile(regionID)
		if err == nil && len(items) > 0 {
			fittingProfile = &zkillboard.RegionDemandProfile{
				RegionID: regionID,
				Items:    make(map[int32]*zkillboard.ItemDemandProfile),
			}
			for _, item := range items {
				fittingProfile.Items[item.TypeID] = &zkillboard.ItemDemandProfile{
					TypeID:         item.TypeID,
					TypeName:       item.TypeName,
					Category:       item.Category,
					TotalDestroyed: item.TotalDestroyed,
					KillmailCount:  item.KillmailCount,
					AvgPerKillmail: item.AvgPerKillmail,
					EstDailyDemand: item.EstDailyDemand,
				}
			}
		}
	}

	// Get opportunities (with fitting profile if available)
	opportunities, err := analyzer.GetRegionOpportunities(regionID, esiClient, fittingProfile)
	if err != nil {
		writeError(w, 500, fmt.Sprintf("failed to get opportunities: %v", err))
		return
	}

	if opportunities == nil {
		writeError(w, 404, "region not found or no data")
		return
	}

	// Resolve type names + volumes from SDE
	if sdeData != nil {
		resolveTypeInfo := func(opps []zkillboard.TradeOpportunity) {
			for i := range opps {
				if t, ok := sdeData.Types[opps[i].TypeID]; ok {
					if opps[i].TypeName == "" {
						opps[i].TypeName = t.Name
					}
					// FIX #6: Populate Volume (m³) from SDE
					opps[i].Volume = t.Volume
				}
			}
		}
		resolveTypeInfo(opportunities.Ships)
		resolveTypeInfo(opportunities.Modules)
		resolveTypeInfo(opportunities.Ammo)

		// Calculate security class and jumps from Jita
		const jitaSystemID = int32(30000142)

		// Find systems in this region and calculate security distribution
		var highCount, lowCount, nullCount int
		var closestDistance = 999999
		var mainSystemName string

		for _, sys := range sdeData.Systems {
			if sys.RegionID == regionID {
				// Count security types
				if sys.Security >= 0.45 {
					highCount++
				} else if sys.Security > 0.0 {
					lowCount++
				} else {
					nullCount++
				}

				// Find closest system to Jita (using graph if available)
				if sdeData.Universe != nil {
					dist := sdeData.Universe.ShortestPath(jitaSystemID, sys.ID)
					if dist >= 0 && dist < closestDistance {
						closestDistance = dist
						mainSystemName = sys.Name
					}
				}
			}
		}

		// Determine dominant security class
		total := highCount + lowCount + nullCount
		if total > 0 {
			// Build security blocks array
			var blocks []string
			if highCount > 0 {
				blocks = append(blocks, "high")
			}
			if lowCount > 0 {
				blocks = append(blocks, "low")
			}
			if nullCount > 0 {
				blocks = append(blocks, "null")
			}
			opportunities.SecurityBlocks = blocks

			// Dominant class
			if nullCount > highCount && nullCount > lowCount {
				opportunities.SecurityClass = "nullsec"
			} else if lowCount > highCount {
				opportunities.SecurityClass = "lowsec"
			} else if highCount > 0 {
				opportunities.SecurityClass = "highsec"
			} else {
				opportunities.SecurityClass = "nullsec"
			}
		}

		// Set jumps from Jita
		if closestDistance < 999999 {
			opportunities.JumpsFromJita = closestDistance
			opportunities.MainSystem = mainSystemName
		}
	}

	writeJSON(w, opportunities)
}

// handleDemandFittings returns raw fitting demand data for a region.
func (s *Server) handleDemandFittings(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	regionIDStr := r.PathValue("regionID")
	regionIDInt, err := strconv.Atoi(regionIDStr)
	if err != nil {
		writeError(w, 400, "invalid region ID")
		return
	}
	regionID := int32(regionIDInt)

	items, err := s.db.GetFittingDemandProfile(regionID)
	if err != nil {
		writeError(w, 500, fmt.Sprintf("failed to get fitting data: %v", err))
		return
	}

	fresh := s.db.IsFittingProfileFresh(regionID, 2*time.Hour)

	writeJSON(w, map[string]interface{}{
		"region_id":  regionID,
		"items":      items,
		"count":      len(items),
		"from_cache": fresh,
	})
}

// handleDemandRefresh forces a refresh of demand data for all regions.
// Uses NDJSON streaming so the frontend can track progress in real time.
func (s *Server) handleDemandRefresh(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	s.mu.RLock()
	analyzer := s.demandAnalyzer
	s.mu.RUnlock()

	if analyzer == nil {
		writeError(w, 503, "demand analyzer not ready")
		return
	}

	s.mu.RLock()
	esiClient := s.esi
	sdeData := s.sdeData
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	sendProgress := func(msg string) {
		line, _ := json.Marshal(map[string]string{"type": "progress", "message": msg})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	}

	sendProgress("Clearing cache...")
	analyzer.ClearCache()
	log.Printf("[Demand] Cache cleared, starting refresh...")

	sendProgress("Fetching region kill data from zKillboard...")
	zones, err := analyzer.GetHotZones(0)
	if err != nil {
		log.Printf("[Demand] Refresh failed: %v", err)
		line, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		return
	}

	sendProgress(fmt.Sprintf("Saving %d regions...", len(zones)))
	for _, z := range zones {
		if err := s.db.SaveDemandRegion(&db.DemandRegion{
			RegionID:      z.RegionID,
			RegionName:    z.RegionName,
			HotScore:      z.HotScore,
			Status:        z.Status,
			KillsToday:    z.KillsToday,
			KillsBaseline: z.KillsBaseline,
			ISKDestroyed:  z.ISKDestroyed,
			ActivePlayers: z.ActivePlayers,
			TopShips:      z.TopShips,
		}); err != nil {
			log.Printf("[Demand] Failed to save region %d: %v", z.RegionID, err)
		}
	}
	log.Printf("[Demand] Region refresh completed: %d regions", len(zones))

	// Analyze fittings for hot regions (elevated+)
	var hotRegions []zkillboard.RegionHotZone
	for _, z := range zones {
		if z.HotScore >= 1.15 {
			hotRegions = append(hotRegions, z)
		}
	}
	if len(hotRegions) > 0 && esiClient != nil && sdeData != nil {
		sendProgress(fmt.Sprintf("Analyzing killmail fittings for %d hot regions...", len(hotRegions)))
		for i, z := range hotRegions {
			sendProgress(fmt.Sprintf("Analyzing fittings: %s (%d/%d)...", z.RegionName, i+1, len(hotRegions)))
			profile, err := analyzer.AnalyzeRegionFittings(z.RegionID, esiClient, sdeData, 100)
			if err != nil {
				log.Printf("[Demand] Fitting analysis failed for region %d: %v", z.RegionID, err)
				continue
			}
			var dbItems []db.FittingDemandItem
			for _, item := range profile.Items {
				dbItems = append(dbItems, db.FittingDemandItem{
					RegionID:       z.RegionID,
					TypeID:         item.TypeID,
					TypeName:       item.TypeName,
					Category:       item.Category,
					TotalDestroyed: item.TotalDestroyed,
					KillmailCount:  item.KillmailCount,
					AvgPerKillmail: item.AvgPerKillmail,
					EstDailyDemand: item.EstDailyDemand,
					SampledKills:   profile.SampledKills,
					TotalKills24h:  profile.TotalKills24h,
				})
			}
			if err := s.db.SaveFittingDemandProfile(z.RegionID, dbItems); err != nil {
				log.Printf("[Demand] Failed to save fitting profile for region %d: %v", z.RegionID, err)
			}
		}
		log.Printf("[Demand] Fitting analysis completed for %d regions", len(hotRegions))
	}

	line, _ := json.Marshal(map[string]interface{}{
		"type":    "result",
		"status":  "completed",
		"regions": len(zones),
	})
	fmt.Fprintf(w, "%s\n", line)
	flusher.Flush()
}

// --- PLEX+ ---

func (s *Server) buildPLEXDashboard(salesTax, brokerFee float64, nes engine.NESPrices, omegaUSD float64) (engine.PLEXDashboard, error) {
	// 1) PLEX orders from Global PLEX Market (region 19000001)
	plexOrders, plexErr := s.esi.FetchRegionOrdersByType(engine.GlobalPLEXRegionID, engine.PLEXTypeID)
	if plexErr != nil {
		log.Printf("[PLEX] Failed to fetch global PLEX orders: %v", plexErr)
	}

	// 2) Related items (Extractor, Injector) from Jita
	// MPTC market is disabled by CCP, so we do not build tradable-market paths for it.
	relatedTypes := []int32{engine.SkillExtractorTypeID, engine.LargeSkillInjTypeID}
	relatedOrders := make(map[int32][]esi.MarketOrder, len(relatedTypes))
	for _, tid := range relatedTypes {
		orders, err := s.esi.FetchRegionOrdersByType(engine.JitaRegionID, tid)
		if err != nil {
			log.Printf("[PLEX] Failed to fetch type %d orders: %v", tid, err)
			continue
		}
		relatedOrders[tid] = orders
	}

	// 3) PLEX price history from Global PLEX Market
	history, histErr := s.esi.FetchMarketHistory(engine.GlobalPLEXRegionID, engine.PLEXTypeID)
	if histErr != nil {
		log.Printf("[PLEX] Failed to fetch history: %v", histErr)
	}

	// 4) Related item histories for fill-time estimation
	historyTypes := []int32{engine.SkillExtractorTypeID, engine.LargeSkillInjTypeID}
	relatedHistory := make(map[int32][]esi.HistoryEntry, len(historyTypes))
	for _, tid := range historyTypes {
		entries, err := s.esi.FetchMarketHistory(engine.JitaRegionID, tid)
		if err != nil {
			log.Printf("[PLEX] Failed to fetch history for type %d: %v", tid, err)
			continue
		}
		relatedHistory[tid] = entries
	}

	// 5) Cross-hub orders: 2 items × 3 non-Jita regions
	// Jita orders are already in relatedOrders, so we only need Amarr, Dodixie, Rens.
	crossHubRegions := []int32{10000043, 10000032, 10000030} // Amarr, Dodixie, Rens
	crossHubOrders := make(map[int32]map[int32][]esi.MarketOrder, len(relatedTypes))
	for _, tid := range relatedTypes {
		for _, rid := range crossHubRegions {
			orders, err := s.esi.FetchRegionOrdersByType(rid, tid)
			if err != nil {
				log.Printf("[PLEX] Failed to fetch cross-hub type %d region %d: %v", tid, rid, err)
				continue
			}
			if crossHubOrders[tid] == nil {
				crossHubOrders[tid] = make(map[int32][]esi.MarketOrder)
			}
			crossHubOrders[tid][rid] = orders
		}
	}

	// Include Jita orders in cross-hub map for comparison.
	for tid, orders := range relatedOrders {
		if crossHubOrders[tid] == nil {
			crossHubOrders[tid] = make(map[int32][]esi.MarketOrder)
		}
		crossHubOrders[tid][engine.JitaRegionID] = orders
	}

	log.Printf("[PLEX] Global orders: %d, history: %d, related types: %d, related histories: %d, cross-hub types: %d",
		len(plexOrders), len(history), len(relatedOrders), len(relatedHistory), len(crossHubOrders))

	// If ESI is fully unavailable, prefer stale cache instead of returning an empty dashboard.
	if len(plexOrders) == 0 && len(relatedOrders) == 0 && len(history) == 0 {
		return engine.PLEXDashboard{}, fmt.Errorf("ESI unavailable: no PLEX market data")
	}

	dashboard := engine.ComputePLEXDashboard(plexOrders, relatedOrders, history, relatedHistory, salesTax, brokerFee, nes, omegaUSD, crossHubOrders)
	return dashboard, nil
}

func (s *Server) handlePLEXDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.isReady() {
		writeError(w, 503, "SDE not loaded yet")
		return
	}

	q := r.URL.Query()
	salesTax := 3.6
	brokerFee := 1.0
	if v, err := strconv.ParseFloat(q.Get("sales_tax"), 64); err == nil && v >= 0 && v <= 100 {
		salesTax = v
	}
	if v, err := strconv.ParseFloat(q.Get("broker_fee"), 64); err == nil && v >= 0 && v <= 100 {
		brokerFee = v
	}

	// NES PLEX prices — user-overridable, 0 = use default
	var nes engine.NESPrices
	if v, err := strconv.Atoi(q.Get("nes_extractor")); err == nil && v > 0 {
		nes.ExtractorPLEX = v
	}
	if v, err := strconv.Atoi(q.Get("nes_omega")); err == nil && v > 0 {
		nes.OmegaPLEX = v
	}

	// Omega USD price for ISK/USD comparison (0 = skip)
	var omegaUSD float64
	if v, err := strconv.ParseFloat(q.Get("omega_usd"), 64); err == nil && v > 0 {
		omegaUSD = v
	}

	log.Printf("[API] PLEX Dashboard: salesTax=%.1f, brokerFee=%.1f, nes=%+v, omegaUSD=%.2f", salesTax, brokerFee, nes, omegaUSD)

	// Check cache (5 min TTL, keyed by user params)
	cacheKey := fmt.Sprintf("%.2f_%.2f_%d_%d_%.2f", salesTax, brokerFee, nes.ExtractorPLEX, nes.OmegaPLEX, omegaUSD)
	if cached, ok := s.getPLEXCache(cacheKey, plexCacheTTL); ok {
		log.Printf("[PLEX] Serving fresh cache")
		writeJSON(w, cached)
		return
	}
	// Safety for tests/manual Server{} construction.
	if s.plexBuildSem == nil {
		s.plexCacheMu.Lock()
		if s.plexBuildSem == nil {
			s.plexBuildSem = make(chan struct{}, 1)
		}
		s.plexCacheMu.Unlock()
	}

	value, err, shared := s.plexBuildGroup.Do(cacheKey, func() (interface{}, error) {
		// Another request may have already populated cache while we were queued.
		if cached, ok := s.getPLEXCache(cacheKey, plexCacheTTL); ok {
			return cached, nil
		}

		s.plexBuildSem <- struct{}{}
		defer func() { <-s.plexBuildSem }()

		dashboard, buildErr := s.buildPLEXDashboard(salesTax, brokerFee, nes, omegaUSD)
		if buildErr != nil {
			if stale, ok := s.getPLEXCache(cacheKey, plexStaleCacheTTL); ok {
				log.Printf("[PLEX] Using stale cache due ESI issues: %v", buildErr)
				return stale, nil
			}
			return nil, buildErr
		}

		s.setPLEXCache(cacheKey, dashboard)
		return dashboard, nil
	})
	if err != nil {
		writeError(w, 502, fmt.Sprintf("failed to fetch PLEX dashboard: %v", err))
		return
	}
	dashboard, ok := value.(engine.PLEXDashboard)
	if !ok {
		writeError(w, 500, "unexpected PLEX dashboard type")
		return
	}
	if shared {
		log.Printf("[PLEX] Shared in-flight dashboard build")
	}
	writeJSON(w, dashboard)
}

// ============================================================
// Corporation Handlers
// ============================================================

// handleAuthRoles returns the character's corporation roles and director status.
func (s *Server) handleAuthRoles(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	characterID, allScope, err := parseAuthScope(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, false)
	if err != nil {
		if strings.Contains(err.Error(), "not logged in") {
			writeError(w, 401, err.Error())
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}
	sess := selectedSessions[0]
	token, err := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
	if err != nil {
		writeError(w, 401, err.Error())
		return
	}

	// Fetch roles and corp ID in parallel
	var roles *esi.CharacterRolesResponse
	var corpID int32
	var rolesErr, corpErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		roles, rolesErr = s.esi.GetCharacterRoles(sess.CharacterID, token)
	}()
	go func() {
		defer wg.Done()
		corpID, corpErr = s.esi.GetCharacterCorporationID(sess.CharacterID)
	}()
	wg.Wait()

	result := corp.CharacterRoles{
		CorporationID: corpID,
	}

	if rolesErr == nil && roles != nil {
		result.Roles = roles.Roles
		for _, role := range roles.Roles {
			if role == "Director" || role == "CEO" {
				result.IsDirector = true
				break
			}
		}
	}
	if corpErr != nil {
		log.Printf("[CORP] Failed to fetch corp ID: %v", corpErr)
	}

	writeJSON(w, result)
}

// corpProvider returns the appropriate CorpDataProvider based on the ?mode= query param.
func (s *Server) corpProvider(r *http.Request) (corp.CorpDataProvider, error) {
	mode := r.URL.Query().Get("mode")
	if mode == "live" {
		userID := userIDFromRequest(r)
		characterID, allScope, err := parseAuthScope(r)
		if err != nil {
			return nil, err
		}
		selectedSessions, err := s.authSessionsForScope(userID, characterID, allScope, false)
		if err != nil {
			return nil, fmt.Errorf("not logged in: %w", err)
		}
		sess := selectedSessions[0]
		token, err := s.sessions.EnsureValidTokenForUserCharacter(s.sso, userID, sess.CharacterID)
		if err != nil {
			return nil, fmt.Errorf("not logged in: %w", err)
		}
		corpID, err := s.esi.GetCharacterCorporationID(sess.CharacterID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve corporation: %w", err)
		}
		s.mu.RLock()
		sdeData := s.sdeData
		s.mu.RUnlock()
		return corp.NewESICorpProvider(s.esi, sdeData, token, corpID, sess.CharacterID), nil
	}
	// Default: demo mode
	if s.demoCorpProvider == nil {
		return nil, fmt.Errorf("demo data not ready (SDE still loading)")
	}
	return s.demoCorpProvider, nil
}

func (s *Server) handleCorpDashboard(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	// Fetch adjusted prices for ISK estimation (mining ores, industry products).
	// Non-blocking: if prices fail, dashboard still works with zero ISK estimates.
	var prices corp.PriceMap
	if provider.IsDemo() && s.demoCorpProvider != nil {
		prices = s.demoCorpProvider.DemoPrices()
	} else {
		s.mu.RLock()
		ia := s.industryAnalyzer
		s.mu.RUnlock()
		if ia != nil {
			if adjusted, err := s.esi.GetAllAdjustedPrices(ia.IndustryCache); err == nil {
				prices = make(corp.PriceMap, len(adjusted))
				for k, v := range adjusted {
					prices[k] = v
				}
			} else {
				log.Printf("[CORP] Failed to fetch adjusted prices: %v (ISK estimates will be zero)", err)
			}
		}
	}

	dashboard, err := corp.BuildDashboard(provider, prices)
	if err != nil {
		writeError(w, 500, fmt.Sprintf("dashboard build failed: %v", err))
		return
	}

	writeJSON(w, dashboard)
}

func (s *Server) handleCorpMembers(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	members, err := provider.GetMembers()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, members)
}

func (s *Server) handleCorpWallets(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	wallets, err := provider.GetWallets()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, wallets)
}

func (s *Server) handleCorpJournal(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	division := 1
	if d := r.URL.Query().Get("division"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v >= 1 && v <= 7 {
			division = v
		}
	}
	days := 90
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			days = v
		}
	}

	journal, err := provider.GetJournal(division, days)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, journal)
}

func (s *Server) handleCorpOrders(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	orders, err := provider.GetOrders()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, orders)
}

func (s *Server) handleCorpIndustry(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	jobs, err := provider.GetIndustryJobs()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, jobs)
}

func (s *Server) handleCorpMining(w http.ResponseWriter, r *http.Request) {
	provider, err := s.corpProvider(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	entries, err := provider.GetMiningLedger()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, entries)
}
