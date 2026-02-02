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
)

const sdeURL = "https://developers.eveonline.com/static-data/eve-online-static-data-latest-jsonl.zip"

// Data holds all parsed SDE data.
type Data struct {
	Systems    map[int32]*SolarSystem // systemID -> system
	SystemByName map[string]int32     // lowercase name -> systemID
	SystemNames []string              // all system names for autocomplete
	Types      map[int32]*ItemType    // typeID -> type
	Stations   map[int64]*Station     // stationID -> station
	Universe   *graph.Universe
}

// SolarSystem represents an EVE solar system from the SDE.
type SolarSystem struct {
	ID       int32
	Name     string
	RegionID int32
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
		fmt.Println("Downloading SDE data...")
		if err := downloadFile(zipPath, sdeURL); err != nil {
			return nil, fmt.Errorf("download SDE: %w", err)
		}
		fmt.Println("Extracting SDE data...")
		if err := extractZip(zipPath, extractDir); err != nil {
			return nil, fmt.Errorf("extract SDE: %w", err)
		}
	}

	data := &Data{
		Systems:      make(map[int32]*SolarSystem),
		SystemByName: make(map[string]int32),
		Types:        make(map[int32]*ItemType),
		Stations:     make(map[int64]*Station),
		Universe:     graph.NewUniverse(),
	}

	fmt.Println("Loading solar systems...")
	if err := data.loadSystems(extractDir); err != nil {
		return nil, err
	}
	fmt.Println("Loading item types...")
	if err := data.loadTypes(extractDir); err != nil {
		return nil, err
	}
	fmt.Println("Loading stations...")
	if err := data.loadStations(extractDir); err != nil {
		return nil, err
	}
	fmt.Println("Loading stargates...")
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

	fmt.Printf("SDE loaded: %d systems, %d types, %d stations\n",
		len(data.Systems), len(data.Types), len(data.Stations))
	return data, nil
}

func (d *Data) loadSystems(dir string) error {
	return readJSONL(dir, "mapSolarSystems", func(raw json.RawMessage) error {
		var s struct {
			Key      int32             `json:"_key"`
			Name     map[string]string `json:"name"`
			RegionID int32             `json:"regionID"`
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		name := s.Name["en"]
		if name == "" {
			return nil
		}
		d.Systems[s.Key] = &SolarSystem{
			ID: s.Key, Name: name, RegionID: s.RegionID,
		}
		d.SystemByName[strings.ToLower(name)] = s.Key
		d.SystemNames = append(d.SystemNames, name)
		d.Universe.SetRegion(s.Key, s.RegionID)
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
		fmt.Printf("Warning: SDE file %s.jsonl not found, skipping\n", baseName)
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

	for _, f := range r.File {
		path := filepath.Join(dst, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(path)
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
