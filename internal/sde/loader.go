package sde

import (
	"archive/zip"
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"eve-flipper/internal/graph"
	"eve-flipper/internal/logger"
)

const sdeURL = "https://developers.eveonline.com/static-data/eve-online-static-data-latest-jsonl.zip"

const (
	sdeDownloadAttempts        = 3
	sdeDownloadTimeout         = 5 * time.Minute
	sdeDownloadTLSHandshake    = 60 * time.Second
	sdeDownloadResponseTimeout = 2 * time.Minute
)

// Data holds all parsed SDE data.
type Data struct {
	Systems      map[int32]*SolarSystem // systemID -> system
	SystemByName map[string]int32       // lowercase name -> systemID
	SystemNames  []string               // all system names for autocomplete
	Regions      map[int32]*Region      // regionID -> region
	RegionByName map[string]int32       // lowercase name -> regionID
	Types        map[int32]*ItemType    // typeID -> type
	Groups       map[int32]*ItemGroup   // groupID -> group metadata
	Contraband   map[int32]bool         // typeID -> listed in contrabandTypes
	Stations     map[int64]*Station     // stationID -> station
	Universe     *graph.Universe
	Industry     *IndustryData // blueprints, reprocessing, etc.

	shipTypesMissingPackagedVolume map[int32]bool
}

// Region represents an EVE region from the SDE.
type Region struct {
	ID   int32
	Name string
}

// SolarSystem represents an EVE solar system from the SDE.
type SolarSystem struct {
	ID       int32
	Name     string
	RegionID int32
	Security float64 // 0.0 (null) to 1.0 (highsec); highsec >= 0.45
}

// ItemType represents a market-tradeable item type from the SDE.
type ItemType struct {
	ID           int32
	Name         string
	Volume       float64 // packaged volume in m³
	GroupID      int32   // item group (for categorization: rigs, ships, modules, etc.)
	CategoryID   int32   // item category (6=Ships, 7=Modules, 20=Implants, etc.)
	IsRig        bool    // derived from group metadata
	IsContraband bool    // listed in contrabandTypes
}

// ItemGroup represents group-level SDE metadata used for type classification.
type ItemGroup struct {
	ID         int32
	Name       string
	CategoryID int32
	IsRig      bool
}

// Station represents an NPC station from the SDE.
type Station struct {
	ID       int64
	Name     string
	SystemID int32
}

// Load downloads (if needed) and parses the SDE.
func Load(dataDir string) (*Data, error) {
	zipPath := filepath.Join(dataDir, "sde.zip")
	extractDir := filepath.Join(dataDir, "sde")

	if err := ensureSDEExtracted(dataDir, zipPath, extractDir); err != nil {
		return nil, err
	}

	data := &Data{
		Systems:      make(map[int32]*SolarSystem),
		SystemByName: make(map[string]int32),
		Regions:      make(map[int32]*Region),
		RegionByName: make(map[string]int32),
		Types:        make(map[int32]*ItemType),
		Groups:       make(map[int32]*ItemGroup),
		Contraband:   make(map[int32]bool),
		Stations:     make(map[int64]*Station),
		Universe:     graph.NewUniverse(),

		shipTypesMissingPackagedVolume: make(map[int32]bool),
	}

	logger.Info("SDE", "Loading regions...")
	if err := data.loadRegions(extractDir); err != nil {
		return nil, err
	}
	logger.Info("SDE", "Loading solar systems...")
	if err := data.loadSystems(extractDir); err != nil {
		return nil, err
	}
	logger.Info("SDE", "Loading item types...")
	if err := data.loadTypes(extractDir); err != nil {
		return nil, err
	}
	logger.Info("SDE", "Loading stations...")
	if err := data.loadStations(extractDir); err != nil {
		return nil, err
	}
	logger.Info("SDE", "Loading stargates...")
	if err := data.loadStargates(extractDir); err != nil {
		return nil, err
	}

	// Resolve station names from system names
	for _, st := range data.Stations {
		if sys, ok := data.Systems[st.SystemID]; ok {
			st.Name = fmt.Sprintf("Station in %s", sys.Name)
		} else {
			st.Name = fmt.Sprintf("Station %d", st.ID)
		}
	}

	// Load industry data (blueprints, reprocessing)
	logger.Info("SDE", "Loading industry data...")
	industry, err := data.LoadIndustry(extractDir)
	if err != nil {
		return nil, fmt.Errorf("load industry: %w", err)
	}
	data.Industry = industry

	// Initialize BFS path cache now that the universe graph is fully loaded.
	data.Universe.InitPathCache()

	logger.Section("SDE Statistics")
	logger.Stats("Regions", len(data.Regions))
	logger.Stats("Systems", len(data.Systems))
	logger.Stats("Item types", len(data.Types))
	logger.Stats("Stations", len(data.Stations))
	logger.Stats("Blueprints", len(data.Industry.Blueprints))
	return data, nil
}

var requiredSDEJSONLFiles = []string{
	"mapRegions",
	"mapSolarSystems",
	"groups",
	"types",
	"npcStations",
	"mapStargates",
}

func ensureSDEExtracted(dataDir, zipPath, extractDir string) error {
	if err := validateSDEExtractDir(extractDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		logger.Warn("SDE", fmt.Sprintf("Existing SDE extract is incomplete: %v", err))
	}

	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		logger.Info("SDE", "Downloading data... first launch can take a few minutes")
		if err := downloadFile(zipPath, sdeURL); err != nil {
			return fmt.Errorf("download SDE: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("stat SDE zip: %w", err)
	}

	logger.Info("SDE", "Extracting data...")
	return extractSDEAtomically(zipPath, extractDir)
}

func validateSDEExtractDir(extractDir string) error {
	info, err := os.Stat(extractDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("SDE extract path is not a directory: %s", extractDir)
	}
	for _, baseName := range requiredSDEJSONLFiles {
		path, err := findJSONLPath(extractDir, baseName)
		if err != nil {
			return err
		}
		if path == "" {
			return fmt.Errorf("required SDE file %s.jsonl is missing", baseName)
		}
	}
	return nil
}

func extractSDEAtomically(zipPath, extractDir string) error {
	parent := filepath.Dir(extractDir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(parent, ".sde-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := extractZip(zipPath, tempDir); err != nil {
		return fmt.Errorf("extract SDE: %w", err)
	}
	if err := validateSDEExtractDir(tempDir); err != nil {
		return fmt.Errorf("validate extracted SDE: %w", err)
	}

	if _, err := os.Stat(extractDir); err == nil {
		corruptDir := extractDir + ".corrupt." + time.Now().UTC().Format("20060102T150405Z")
		if err := os.Rename(extractDir, corruptDir); err != nil {
			return fmt.Errorf("quarantine incomplete SDE extract: %w", err)
		}
		logger.Warn("SDE", fmt.Sprintf("Moved incomplete SDE extract to %s", corruptDir))
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat SDE extract dir: %w", err)
	}

	if err := os.Rename(tempDir, extractDir); err != nil {
		return fmt.Errorf("activate SDE extract: %w", err)
	}
	return nil
}

// RegionNames returns a map of region ID to region name.
func (d *Data) RegionNames() map[int32]string {
	names := make(map[int32]string, len(d.Regions))
	for id, r := range d.Regions {
		names[id] = r.Name
	}
	return names
}

func (d *Data) loadRegions(dir string) error {
	return readJSONL(dir, "mapRegions", func(raw json.RawMessage) error {
		var r struct {
			Key  int32             `json:"_key"`
			Name map[string]string `json:"name"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		name := r.Name["en"]
		if name == "" {
			return nil
		}
		d.Regions[r.Key] = &Region{
			ID:   r.Key,
			Name: name,
		}
		d.RegionByName[strings.ToLower(name)] = r.Key
		return nil
	})
}

func (d *Data) loadSystems(dir string) error {
	return readJSONL(dir, "mapSolarSystems", func(raw json.RawMessage) error {
		var s struct {
			Key            int32             `json:"_key"`
			Name           map[string]string `json:"name"`
			RegionID       int32             `json:"regionID"`
			Security       float64           `json:"security"`
			SecurityStatus float64           `json:"securityStatus"` // alternate SDE field name
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		name := s.Name["en"]
		if name == "" {
			return nil
		}
		sec := s.Security
		if sec == 0 && s.SecurityStatus != 0 {
			sec = s.SecurityStatus
		}
		d.Systems[s.Key] = &SolarSystem{
			ID: s.Key, Name: name, RegionID: s.RegionID, Security: sec,
		}
		d.SystemByName[strings.ToLower(name)] = s.Key
		d.SystemNames = append(d.SystemNames, name)
		d.Universe.SetRegion(s.Key, s.RegionID)
		d.Universe.SetSecurity(s.Key, sec)
		return nil
	})
}

func (d *Data) loadTypes(dir string) error {
	// First load groups to get category mapping and data-driven rig classification.
	groupCategories := make(map[int32]int32) // groupID -> categoryID
	groupRig := make(map[int32]bool)         // groupID -> is rig group
	_, err := readOptionalJSONL(dir, "contrabandTypes", func(raw json.RawMessage) error {
		var c struct {
			Key int32 `json:"_key"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		if c.Key > 0 {
			d.Contraband[c.Key] = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("load contraband types: %w", err)
	}

	err = readJSONL(dir, "groups", func(raw json.RawMessage) error {
		var g struct {
			Key        int32             `json:"_key"`
			Name       map[string]string `json:"name"`
			CategoryID int32             `json:"categoryID"`
		}
		if err := json.Unmarshal(raw, &g); err != nil {
			return err
		}
		nameEN := strings.TrimSpace(g.Name["en"])
		groupCategories[g.Key] = g.CategoryID
		groupRig[g.Key] = isRigGroupName(g.CategoryID, nameEN)
		d.Groups[g.Key] = &ItemGroup{
			ID:         g.Key,
			Name:       nameEN,
			CategoryID: g.CategoryID,
			IsRig:      groupRig[g.Key],
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("load groups: %w", err)
	}

	// Then load types
	return readJSONL(dir, "types", func(raw json.RawMessage) error {
		var t struct {
			Key            int32             `json:"_key"`
			Name           map[string]string `json:"name"`
			Volume         float64           `json:"volume"`
			PackagedVolume float64           `json:"packagedVolume"`
			Published      bool              `json:"published"`
			MarketGroupID  *int32            `json:"marketGroupID"`
			GroupID        int32             `json:"groupID"`
		}
		if err := json.Unmarshal(raw, &t); err != nil {
			return err
		}
		if !t.Published || t.MarketGroupID == nil {
			return nil
		}
		name := t.Name["en"]
		if name == "" {
			return nil
		}
		categoryID := groupCategories[t.GroupID]
		vol := t.PackagedVolume
		if vol == 0 {
			vol = t.Volume
			if categoryID == 6 {
				d.shipTypesMissingPackagedVolume[t.Key] = true
			}
		}
		d.Types[t.Key] = &ItemType{
			ID:           t.Key,
			Name:         name,
			Volume:       vol,
			GroupID:      t.GroupID,
			CategoryID:   categoryID,
			IsRig:        groupRig[t.GroupID],
			IsContraband: d.Contraband[t.Key],
		}
		return nil
	})
}

func isRigGroupName(categoryID int32, groupName string) bool {
	if categoryID != 7 {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(groupName))
	// In SDE, rig groups in category Modules are consistently prefixed with "Rig".
	return strings.HasPrefix(name, "rig")
}

func (d *Data) loadStations(dir string) error {
	// npcStations.jsonl has _key (stationID), solarSystemID, typeID, ownerID, etc.
	// Station names are not in this file — we'll build them from system + owner info.
	return readJSONL(dir, "npcStations", func(raw json.RawMessage) error {
		var s struct {
			Key           int64 `json:"_key"`
			SolarSystemID int32 `json:"solarSystemID"`
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		// Name will be resolved later from system name
		d.Stations[s.Key] = &Station{
			ID: s.Key, Name: "", SystemID: s.SolarSystemID,
		}
		return nil
	})
}

func (d *Data) loadStargates(dir string) error {
	return readJSONL(dir, "mapStargates", func(raw json.RawMessage) error {
		var g struct {
			SolarSystemID int32 `json:"solarSystemID"`
			Destination   struct {
				SolarSystemID int32 `json:"solarSystemID"`
			} `json:"destination"`
		}
		if err := json.Unmarshal(raw, &g); err != nil {
			return err
		}
		if g.SolarSystemID != 0 && g.Destination.SolarSystemID != 0 {
			d.Universe.AddGate(g.SolarSystemID, g.Destination.SolarSystemID)
		}
		return nil
	})
}

// readJSONL finds and reads a .jsonl file by base name from the extracted SDE directory.
func readJSONL(dir, baseName string, fn func(json.RawMessage) error) error {
	filePath, err := findJSONLPath(dir, baseName)
	if err != nil {
		return err
	}
	if filePath == "" {
		logger.Warn("SDE", fmt.Sprintf("File %s.jsonl not found, skipping", baseName))
		return nil
	}
	return readJSONLFile(filePath, fn)
}

func readOptionalJSONL(dir, baseName string, fn func(json.RawMessage) error) (bool, error) {
	filePath, err := findJSONLPath(dir, baseName)
	if err != nil {
		return false, err
	}
	if filePath == "" {
		return false, nil
	}
	return true, readJSONLFile(filePath, fn)
}

func findJSONLPath(dir, baseName string) (string, error) {
	var filePath string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		name := strings.TrimSuffix(info.Name(), ".jsonl")
		if strings.EqualFold(name, baseName) {
			filePath = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return "", err
	}
	return filePath, nil
}

func readJSONLFile(filePath string, fn func(json.RawMessage) error) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if err := fn(json.RawMessage(line)); err != nil {
			continue // skip malformed lines
		}
	}
	return scanner.Err()
}

func downloadFile(dst, url string) error {
	os.MkdirAll(filepath.Dir(dst), 0755)

	client := &http.Client{
		Timeout: sdeDownloadTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   45 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   sdeDownloadTLSHandshake,
			ResponseHeaderTimeout: sdeDownloadResponseTimeout,
			ExpectContinueTimeout: 10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	var lastErr error
	for attempt := 1; attempt <= sdeDownloadAttempts; attempt++ {
		if attempt > 1 {
			delay := time.Duration(attempt*attempt) * time.Second
			logger.Warn("SDE", fmt.Sprintf("Retrying SDE download in %s (attempt %d/%d)", delay, attempt, sdeDownloadAttempts))
			time.Sleep(delay)
		}
		if err := downloadFileOnce(client, dst, url); err != nil {
			lastErr = err
			logger.Warn("SDE", fmt.Sprintf("SDE download attempt %d/%d failed: %v", attempt, sdeDownloadAttempts, err))
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown download error")
	}
	return fmt.Errorf("%w; retry later or download the SDE manually into %s", lastErr, dst)
}

func downloadFileOnce(client *http.Client, dst, url string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "eve-flipper/1.0 (github.com)")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmp := dst + ".tmp"
	_ = os.Remove(tmp)
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func extractZip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// Resolve destination to an absolute path for zip slip prevention
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("resolve extract dir: %w", err)
	}

	for _, f := range r.File {
		fpath := filepath.Join(dstAbs, f.Name)

		// Zip slip guard: ensure the resolved path stays within dst
		if rel, err := filepath.Rel(dstAbs, fpath); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("illegal zip entry path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(fpath)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
