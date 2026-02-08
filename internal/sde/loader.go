package sde

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"eve-flipper/internal/graph"
	"eve-flipper/internal/logger"
)

const sdeURL = "https://developers.eveonline.com/static-data/eve-online-static-data-latest-jsonl.zip"

// Data holds all parsed SDE data.
type Data struct {
	Systems      map[int32]*SolarSystem // systemID -> system
	SystemByName map[string]int32       // lowercase name -> systemID
	SystemNames  []string               // all system names for autocomplete
	Regions      map[int32]*Region      // regionID -> region
	RegionByName map[string]int32       // lowercase name -> regionID
	Types        map[int32]*ItemType    // typeID -> type
	Stations     map[int64]*Station     // stationID -> station
	Universe     *graph.Universe
	Industry     *IndustryData // blueprints, reprocessing, etc.
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
	ID     int32
	Name   string
	Volume float64 // packaged volume in m³
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

	if _, err := os.Stat(extractDir); os.IsNotExist(err) {
		logger.Info("SDE", "Downloading data...")
		if err := downloadFile(zipPath, sdeURL); err != nil {
			return nil, fmt.Errorf("download SDE: %w", err)
		}
		logger.Info("SDE", "Extracting data...")
		if err := extractZip(zipPath, extractDir); err != nil {
			return nil, fmt.Errorf("extract SDE: %w", err)
		}
	}

	data := &Data{
		Systems:      make(map[int32]*SolarSystem),
		SystemByName: make(map[string]int32),
		Regions:      make(map[int32]*Region),
		RegionByName: make(map[string]int32),
		Types:        make(map[int32]*ItemType),
		Stations:     make(map[int64]*Station),
		Universe:     graph.NewUniverse(),
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
	return readJSONL(dir, "types", func(raw json.RawMessage) error {
		var t struct {
			Key            int32             `json:"_key"`
			Name           map[string]string `json:"name"`
			Volume         float64           `json:"volume"`
			PackagedVolume float64           `json:"packagedVolume"`
			Published      bool              `json:"published"`
			MarketGroupID  *int32            `json:"marketGroupID"`
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
		vol := t.PackagedVolume
		if vol == 0 {
			vol = t.Volume
		}
		d.Types[t.Key] = &ItemType{
			ID: t.Key, Name: name, Volume: vol,
		}
		return nil
	})
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
	// Search for the file recursively
	var filePath string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
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
		return err
	}
	if filePath == "" {
		logger.Warn("SDE", fmt.Sprintf("File %s.jsonl not found, skipping", baseName))
		return nil
	}

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
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
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
