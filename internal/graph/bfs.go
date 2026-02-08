package graph

import "sync"

// pathCacheKey identifies a cached shortest-path query.
type pathCacheKey struct {
	from       int32
	to         int32
	minSecTier int8 // discretized security: 0 = any, 1 = ≥0.45, 2 = ≥0.5, etc.
}

// pathCache is a bounded LRU cache for ShortestPath results.
// EVE universe has ~8000 systems; caching the most frequently queried pairs
// avoids redundant BFS runs during scans (hundreds of results × BFS each).
type pathCache struct {
	mu       sync.RWMutex
	entries  map[pathCacheKey]int
	order    []pathCacheKey // insertion order (oldest first)
	maxSize  int
}

const defaultPathCacheSize = 50_000

func newPathCache(maxSize int) *pathCache {
	if maxSize <= 0 {
		maxSize = defaultPathCacheSize
	}
	return &pathCache{
		entries: make(map[pathCacheKey]int, maxSize),
		order:   make([]pathCacheKey, 0, maxSize),
		maxSize: maxSize,
	}
}

func (pc *pathCache) get(key pathCacheKey) (int, bool) {
	pc.mu.RLock()
	v, ok := pc.entries[key]
	pc.mu.RUnlock()
	return v, ok
}

func (pc *pathCache) put(key pathCacheKey, dist int) {
	pc.mu.Lock()
	if _, exists := pc.entries[key]; exists {
		pc.mu.Unlock()
		return
	}
	// Evict oldest entries if at capacity
	for len(pc.entries) >= pc.maxSize && len(pc.order) > 0 {
		oldest := pc.order[0]
		pc.order = pc.order[1:]
		delete(pc.entries, oldest)
	}
	pc.entries[key] = dist
	pc.order = append(pc.order, key)
	pc.mu.Unlock()
}

// securityTier discretizes minSecurity into a small int for cache keying.
// This avoids floating-point equality issues and keeps the key space small.
func securityTier(minSecurity float64) int8 {
	if minSecurity <= 0 {
		return 0
	}
	// Round to nearest 0.05 step and encode as tier 1-20
	tier := int8(minSecurity*20 + 0.5)
	if tier < 1 {
		tier = 1
	}
	if tier > 20 {
		tier = 20
	}
	return tier
}

// SystemsWithinRadius returns all systems reachable from origin within maxJumps,
// mapped to their distance in jumps.
func (u *Universe) SystemsWithinRadius(origin int32, maxJumps int) map[int32]int {
	return u.SystemsWithinRadiusMinSecurity(origin, maxJumps, 0)
}

// SystemsWithinRadiusMinSecurity returns systems reachable within maxJumps where
// every system on the path has security >= minSecurity. Use minSecurity <= 0 for no filter.
func (u *Universe) SystemsWithinRadiusMinSecurity(origin int32, maxJumps int, minSecurity float64) map[int32]int {
	result := make(map[int32]int)
	result[origin] = 0

	// Ring buffer queue to avoid slice shift overhead.
	queue := make([]int32, 0, 256)
	queue = append(queue, origin)
	head := 0

	for head < len(queue) {
		current := queue[head]
		head++
		// Reclaim memory periodically when head gets far ahead
		if head > 1024 && head > len(queue)/2 {
			remaining := queue[head:]
			queue = make([]int32, len(remaining), len(remaining)+256)
			copy(queue, remaining)
			head = 0
		}

		dist := result[current]
		if dist >= maxJumps {
			continue
		}
		for _, neighbor := range u.Adj[current] {
			if minSecurity > 0 {
				if sec, ok := u.SystemSecurity[neighbor]; !ok || sec < minSecurity {
					continue
				}
			}
			if _, visited := result[neighbor]; !visited {
				result[neighbor] = dist + 1
				queue = append(queue, neighbor)
			}
		}
	}
	return result
}

// ShortestPath returns the shortest jump count between origin and dest using BFS.
// All edges have unit weight (1 jump), so BFS is optimal.
// Results are cached to avoid redundant BFS runs during scans.
// Returns -1 if no path exists.
func (u *Universe) ShortestPath(origin, dest int32) int {
	return u.ShortestPathMinSecurity(origin, dest, 0)
}

// ShortestPathMinSecurity returns the shortest jump count using only systems with
// security >= minSecurity. Uses BFS (all edges are unit weight).
// Results are cached in an LRU cache (up to 50k entries).
// Use minSecurity <= 0 for no filter. Returns -1 if no path exists.
func (u *Universe) ShortestPathMinSecurity(origin, dest int32, minSecurity float64) int {
	if origin == dest {
		return 0
	}
	if minSecurity > 0 {
		if sec, ok := u.SystemSecurity[origin]; ok && sec < minSecurity {
			return -1
		}
		if sec, ok := u.SystemSecurity[dest]; ok && sec < minSecurity {
			return -1
		}
	}

	// Check cache
	tier := securityTier(minSecurity)
	cacheKey := pathCacheKey{from: origin, to: dest, minSecTier: tier}
	if u.pathCacheMu != nil {
		if d, ok := u.pathCacheMu.get(cacheKey); ok {
			return d
		}
		// Also check reverse direction (undirected graph)
		reverseKey := pathCacheKey{from: dest, to: origin, minSecTier: tier}
		if d, ok := u.pathCacheMu.get(reverseKey); ok {
			return d
		}
	}

	d := u.bfs(origin, dest, minSecurity)

	// Store in cache
	if u.pathCacheMu != nil {
		u.pathCacheMu.put(cacheKey, d)
	}

	return d
}

// bfs performs a breadth-first search from origin to dest.
// Uses a ring buffer queue to avoid slice shift overhead.
func (u *Universe) bfs(origin, dest int32, minSecurity float64) int {
	dist := make(map[int32]int, 256)
	dist[origin] = 0

	queue := make([]int32, 0, 256)
	queue = append(queue, origin)
	head := 0

	for head < len(queue) {
		current := queue[head]
		head++
		// Reclaim memory periodically
		if head > 1024 && head > len(queue)/2 {
			remaining := queue[head:]
			queue = make([]int32, len(remaining), len(remaining)+256)
			copy(queue, remaining)
			head = 0
		}

		currentDist := dist[current]

		for _, neighbor := range u.Adj[current] {
			if minSecurity > 0 {
				if sec, ok := u.SystemSecurity[neighbor]; !ok || sec < minSecurity {
					continue
				}
			}
			if _, visited := dist[neighbor]; !visited {
				nd := currentDist + 1
				if neighbor == dest {
					return nd
				}
				dist[neighbor] = nd
				queue = append(queue, neighbor)
			}
		}
	}
	return -1
}

// InitPathCache initializes the shortest-path LRU cache.
// Must be called after the universe graph is fully loaded.
// Safe to call multiple times (idempotent).
func (u *Universe) InitPathCache() {
	if u.pathCacheMu == nil {
		u.pathCacheMu = newPathCache(defaultPathCacheSize)
	}
}

// ClearPathCache discards all cached shortest-path results.
func (u *Universe) ClearPathCache() {
	if u.pathCacheMu != nil {
		u.pathCacheMu.mu.Lock()
		u.pathCacheMu.entries = make(map[pathCacheKey]int, u.pathCacheMu.maxSize)
		u.pathCacheMu.order = u.pathCacheMu.order[:0]
		u.pathCacheMu.mu.Unlock()
	}
}

// RegionsInSet returns the unique region IDs for a set of systems.
func (u *Universe) RegionsInSet(systems map[int32]int) map[int32]bool {
	regions := make(map[int32]bool)
	for sysID := range systems {
		if r, ok := u.SystemRegion[sysID]; ok {
			regions[r] = true
		}
	}
	return regions
}

// SystemsInRegions returns all system IDs that belong to any of the given regions.
// Used for multi-region arbitrage: consider all systems in the region, not just within jump radius.
func (u *Universe) SystemsInRegions(regions map[int32]bool) map[int32]int {
	out := make(map[int32]int)
	for sysID, regionID := range u.SystemRegion {
		if regions[regionID] {
			out[sysID] = 0
		}
	}
	return out
}
