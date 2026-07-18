package sde

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshShipPackagedVolumeCacheQuarantinesCorruptCache(t *testing.T) {
	dataDir := t.TempDir()
	cachePath := filepath.Join(dataDir, shipPackagedVolumeCacheFile)
	if err := os.WriteFile(cachePath, []byte("{not json"), 0644); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}

	data := &Data{
		Types: map[int32]*ItemType{
			608: {ID: 608, Name: "Atron", CategoryID: 6, Volume: 22500},
		},
		shipTypesMissingPackagedVolume: map[int32]bool{608: true},
	}

	result, err := RefreshShipPackagedVolumeCache(dataDir, data, func(typeID int32) (float64, error) {
		if typeID != 608 {
			t.Fatalf("fetch typeID = %d, want 608", typeID)
		}
		return 2500, nil
	})
	if err != nil {
		t.Fatalf("refresh corrupt cache: %v", err)
	}
	if result.CorruptCachePath == "" {
		t.Fatalf("CorruptCachePath empty, result = %#v", result)
	}
	if result.Fetched != 1 || result.Applied != 1 || result.Failed != 0 {
		t.Fatalf("refresh result = %#v, want fetched=1 applied=1 failed=0", result)
	}
	if data.Types[608].Volume != 2500 {
		t.Fatalf("ship volume = %v, want 2500", data.Types[608].Volume)
	}
	if _, err := os.Stat(result.CorruptCachePath); err != nil {
		t.Fatalf("corrupt cache was not moved to %s: %v", result.CorruptCachePath, err)
	}
	if _, _, err := LoadShipPackagedVolumeCache(dataDir); err != nil {
		t.Fatalf("saved cache should be readable: %v", err)
	}
}

func TestRefreshShipPackagedVolumeCacheForTypesSkipsCachedAndDeduplicates(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := SaveShipPackagedVolumeCache(dataDir, map[int32]float64{608: 2500}); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	var fetched []int32
	result, err := RefreshShipPackagedVolumeCacheForTypes(dataDir, []int32{608, 626, 626}, func(typeID int32) (float64, error) {
		fetched = append(fetched, typeID)
		return 10000, nil
	})
	if err != nil {
		t.Fatalf("refresh cache for types: %v", err)
	}
	if result.Missing != 1 || result.Fetched != 1 || result.Failed != 0 {
		t.Fatalf("refresh result = %#v, want missing=1 fetched=1 failed=0", result)
	}
	if len(fetched) != 1 || fetched[0] != 626 {
		t.Fatalf("fetched = %v, want only 626", fetched)
	}
	cache, _, err := LoadShipPackagedVolumeCache(dataDir)
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	if cache[608] != 2500 || cache[626] != 10000 {
		t.Fatalf("cache = %#v, want cached and fetched values", cache)
	}
}
