package main

import (
	"context"
	"fmt"
	"time"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/logger"
	"eve-flipper/internal/sde"
)

const shipPackagedVolumeRefreshTimeout = 45 * time.Second

func prepareShipPackagedVolumes(dataDir string, data *sde.Data) []int32 {
	if data == nil {
		return nil
	}
	result, err := sde.ApplyCachedShipPackagedVolumes(dataDir, data)
	if err != nil {
		logger.Warn("SDE", fmt.Sprintf("Ship packaged-volume cache apply failed: %v", err))
		return data.MissingShipPackagedVolumeTypeIDs()
	}
	if result.CorruptCachePath != "" {
		logger.Warn("SDE", fmt.Sprintf("Moved corrupt ship packaged-volume cache to %s", result.CorruptCachePath))
	}
	if result.Applied > 0 || result.Missing > 0 {
		logger.Info("SDE", fmt.Sprintf(
			"Ship packaged-volume cache: applied=%d missing=%d path=%s",
			result.Applied,
			result.Missing,
			result.CachePath,
		))
	}
	return data.MissingShipPackagedVolumeTypeIDs()
}

func refreshShipPackagedVolumesInBackground(dataDir string, missing []int32, esiClient *esi.Client) {
	if len(missing) == 0 || esiClient == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), shipPackagedVolumeRefreshTimeout)
		defer cancel()

		result, err := sde.RefreshShipPackagedVolumeCacheForTypes(dataDir, missing, func(typeID int32) (float64, error) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			default:
			}
			info, err := esiClient.TypeInfo(typeID)
			if err != nil {
				return 0, err
			}
			return info.PackagedVolume, nil
		})
		if err != nil {
			logger.Warn("SDE", fmt.Sprintf("Ship packaged-volume background refresh failed: %v", err))
			return
		}
		if result.CorruptCachePath != "" {
			logger.Warn("SDE", fmt.Sprintf("Moved corrupt ship packaged-volume cache to %s", result.CorruptCachePath))
		}
		if result.Fetched > 0 || result.Failed > 0 {
			logger.Info("SDE", fmt.Sprintf(
				"Ship packaged-volume background refresh: fetched=%d failed=%d missing=%d path=%s",
				result.Fetched,
				result.Failed,
				result.Missing,
				result.CachePath,
			))
		}
	}()
}
