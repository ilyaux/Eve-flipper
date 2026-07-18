package sde

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const shipPackagedVolumeCacheFile = "ship_packaged_volumes.json"

type ShipPackagedVolumeCache struct {
	UpdatedAt string             `json:"updated_at"`
	Source    string             `json:"source"`
	Volumes   map[string]float64 `json:"volumes"`
}

type ShipPackagedVolumeRefreshResult struct {
	CachePath        string
	CorruptCachePath string
	Cached           int
	Missing          int
	Fetched          int
	Failed           int
	Applied          int
}

func LoadShipPackagedVolumeCache(dataDir string) (map[int32]float64, string, error) {
	path := filepath.Join(dataDir, shipPackagedVolumeCacheFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[int32]float64{}, path, nil
		}
		return nil, path, err
	}
	var cache ShipPackagedVolumeCache
	if err := json.Unmarshal(raw, &cache); err != nil {
		return nil, path, err
	}
	out := make(map[int32]float64, len(cache.Volumes))
	for key, volume := range cache.Volumes {
		typeID, err := strconv.ParseInt(key, 10, 32)
		if err != nil || typeID <= 0 || volume <= 0 {
			continue
		}
		out[int32(typeID)] = volume
	}
	return out, path, nil
}

func loadShipPackagedVolumeCacheForUpdate(dataDir string) (map[int32]float64, string, string, error) {
	cache, path, err := LoadShipPackagedVolumeCache(dataDir)
	if err == nil {
		return cache, path, "", nil
	}
	if !isCorruptShipPackagedVolumeCache(err) {
		return nil, path, "", err
	}
	corruptPath := path + ".corrupt." + time.Now().UTC().Format("20060102T150405Z")
	if renameErr := os.Rename(path, corruptPath); renameErr != nil {
		return nil, path, "", fmt.Errorf("move corrupt ship packaged volume cache: %w", renameErr)
	}
	return map[int32]float64{}, path, corruptPath, nil
}

func isCorruptShipPackagedVolumeCache(err error) bool {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &syntaxErr) || errors.As(err, &typeErr)
}

func SaveShipPackagedVolumeCache(dataDir string, volumes map[int32]float64) (string, error) {
	path := filepath.Join(dataDir, shipPackagedVolumeCacheFile)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return path, err
	}
	cache := ShipPackagedVolumeCache{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "ESI /universe/types/{type_id}.packaged_volume",
		Volumes:   make(map[string]float64, len(volumes)),
	}
	for typeID, volume := range volumes {
		if typeID > 0 && volume > 0 {
			cache.Volumes[strconv.FormatInt(int64(typeID), 10)] = volume
		}
	}
	raw, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return path, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return path, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return path, err
	}
	return path, nil
}

func (d *Data) ApplyShipPackagedVolumes(volumes map[int32]float64) int {
	if d == nil || len(volumes) == 0 {
		return 0
	}
	applied := 0
	for typeID, volume := range volumes {
		if volume <= 0 {
			continue
		}
		if !d.shipTypesMissingPackagedVolume[typeID] {
			continue
		}
		item := d.Types[typeID]
		if item == nil || item.CategoryID != 6 {
			continue
		}
		item.Volume = volume
		delete(d.shipTypesMissingPackagedVolume, typeID)
		applied++
	}
	return applied
}

func (d *Data) MissingShipPackagedVolumeTypeIDs() []int32 {
	if d == nil || len(d.shipTypesMissingPackagedVolume) == 0 {
		return nil
	}
	ids := make([]int32, 0, len(d.shipTypesMissingPackagedVolume))
	for typeID := range d.shipTypesMissingPackagedVolume {
		ids = append(ids, typeID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func ApplyCachedShipPackagedVolumes(dataDir string, data *Data) (ShipPackagedVolumeRefreshResult, error) {
	cache, path, corruptPath, err := loadShipPackagedVolumeCacheForUpdate(dataDir)
	if err != nil {
		return ShipPackagedVolumeRefreshResult{CachePath: path}, fmt.Errorf("load ship packaged volume cache: %w", err)
	}
	result := ShipPackagedVolumeRefreshResult{
		CachePath:        path,
		CorruptCachePath: corruptPath,
		Cached:           len(cache),
	}
	if data != nil {
		result.Applied = data.ApplyShipPackagedVolumes(cache)
		result.Missing = len(data.MissingShipPackagedVolumeTypeIDs())
	}
	return result, nil
}

func RefreshShipPackagedVolumeCacheForTypes(dataDir string, typeIDs []int32, fetch func(typeID int32) (float64, error)) (ShipPackagedVolumeRefreshResult, error) {
	cache, path, corruptPath, err := loadShipPackagedVolumeCacheForUpdate(dataDir)
	if err != nil {
		return ShipPackagedVolumeRefreshResult{CachePath: path}, fmt.Errorf("load ship packaged volume cache: %w", err)
	}
	result := ShipPackagedVolumeRefreshResult{
		CachePath:        path,
		CorruptCachePath: corruptPath,
		Cached:           len(cache),
	}
	missing := make([]int32, 0, len(typeIDs))
	seen := make(map[int32]bool, len(typeIDs))
	for _, typeID := range typeIDs {
		if typeID <= 0 || seen[typeID] {
			continue
		}
		seen[typeID] = true
		if cache[typeID] <= 0 {
			missing = append(missing, typeID)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	result.Missing = len(missing)
	if len(missing) == 0 || fetch == nil {
		return result, nil
	}

	fetched, failed := fetchShipPackagedVolumes(missing, fetch)
	result.Failed = failed
	for typeID, volume := range fetched {
		cache[typeID] = volume
		result.Fetched++
	}
	if result.Fetched > 0 {
		if _, err := SaveShipPackagedVolumeCache(dataDir, cache); err != nil {
			return result, fmt.Errorf("save ship packaged volume cache: %w", err)
		}
	}
	return result, nil
}

func RefreshShipPackagedVolumeCache(dataDir string, data *Data, fetch func(typeID int32) (float64, error)) (ShipPackagedVolumeRefreshResult, error) {
	cache, path, corruptPath, err := loadShipPackagedVolumeCacheForUpdate(dataDir)
	if err != nil {
		return ShipPackagedVolumeRefreshResult{CachePath: path}, fmt.Errorf("load ship packaged volume cache: %w", err)
	}

	result := ShipPackagedVolumeRefreshResult{
		CachePath:        path,
		CorruptCachePath: corruptPath,
		Cached:           len(cache),
		Applied:          data.ApplyShipPackagedVolumes(cache),
	}
	missing := data.MissingShipPackagedVolumeTypeIDs()
	result.Missing = len(missing)
	if len(missing) == 0 || fetch == nil {
		return result, nil
	}

	fetched, failed := fetchShipPackagedVolumes(missing, fetch)
	result.Failed = failed
	for typeID, volume := range fetched {
		cache[typeID] = volume
		result.Fetched++
	}
	result.Applied += data.ApplyShipPackagedVolumes(cache)
	if result.Fetched > 0 {
		if _, err := SaveShipPackagedVolumeCache(dataDir, cache); err != nil {
			return result, fmt.Errorf("save ship packaged volume cache: %w", err)
		}
	}
	return result, nil
}

func fetchShipPackagedVolumes(typeIDs []int32, fetch func(typeID int32) (float64, error)) (map[int32]float64, int) {
	type fetchResult struct {
		typeID int32
		volume float64
		err    error
	}
	jobs := make(chan int32)
	results := make(chan fetchResult, len(typeIDs))
	workers := 8
	if len(typeIDs) < workers {
		workers = len(typeIDs)
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for typeID := range jobs {
				volume, err := fetch(typeID)
				results <- fetchResult{typeID: typeID, volume: volume, err: err}
			}
		}()
	}
	go func() {
		for _, typeID := range typeIDs {
			jobs <- typeID
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	fetched := make(map[int32]float64)
	failed := 0
	for row := range results {
		if row.err != nil || row.volume <= 0 {
			failed++
			continue
		}
		fetched[row.typeID] = row.volume
	}
	return fetched, failed
}
