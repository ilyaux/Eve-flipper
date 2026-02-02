package db

// GetStation loads a station name from the DB cache.
func (d *DB) GetStation(locationID int64) (string, bool) {
	var name string
	err := d.sql.QueryRow("SELECT name FROM station_cache WHERE location_id = ?", locationID).Scan(&name)
	if err != nil {
		return "", false
	}
	return name, true
}

// SetStation saves a station name to the DB cache.
func (d *DB) SetStation(locationID int64, name string) {
	d.sql.Exec("INSERT OR REPLACE INTO station_cache (location_id, name) VALUES (?, ?)", locationID, name)
}
