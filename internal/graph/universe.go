package graph

// Universe holds the adjacency list of solar systems connected by stargates,
// plus mappings from system to region/constellation and security.
type Universe struct {
	// Adj maps systemID -> list of neighboring systemIDs
	Adj map[int32][]int32
	// SystemRegion maps systemID -> regionID
	SystemRegion map[int32]int32
	// SystemSecurity maps systemID -> security (0.0 null to 1.0 highsec); highsec >= 0.45
	SystemSecurity map[int32]float64
	// pathCacheMu is an LRU cache for ShortestPath results.
	// Initialized lazily via InitPathCache().
	pathCacheMu *pathCache
}

// NewUniverse creates an empty Universe with initialized maps.
func NewUniverse() *Universe {
	return &Universe{
		Adj:            make(map[int32][]int32),
		SystemRegion:   make(map[int32]int32),
		SystemSecurity: make(map[int32]float64),
	}
}

// AddGate adds a bidirectional stargate connection.
func (u *Universe) AddGate(fromSystem, toSystem int32) {
	u.Adj[fromSystem] = append(u.Adj[fromSystem], toSystem)
}

// SetRegion associates a system with a region.
func (u *Universe) SetRegion(systemID, regionID int32) {
	u.SystemRegion[systemID] = regionID
}

// SetSecurity sets the security level for a system (0.0–1.0).
func (u *Universe) SetSecurity(systemID int32, security float64) {
	u.SystemSecurity[systemID] = security
}
