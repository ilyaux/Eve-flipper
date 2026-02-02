package graph

// Universe holds the adjacency list of solar systems connected by stargates,
// plus mappings from system to region/constellation.
type Universe struct {
	// Adj maps systemID -> list of neighboring systemIDs
	Adj map[int32][]int32
	// SystemRegion maps systemID -> regionID
	SystemRegion map[int32]int32
}

// NewUniverse creates an empty Universe with initialized maps.
func NewUniverse() *Universe {
	return &Universe{
		Adj:          make(map[int32][]int32),
		SystemRegion: make(map[int32]int32),
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
