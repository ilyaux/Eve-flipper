package graph

import "container/heap"

// SystemsWithinRadius returns all systems reachable from origin within maxJumps,
// mapped to their distance in jumps.
func (u *Universe) SystemsWithinRadius(origin int32, maxJumps int) map[int32]int {
	result := make(map[int32]int)
	result[origin] = 0

	queue := []int32{origin}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		dist := result[current]
		if dist >= maxJumps {
			continue
		}
		for _, neighbor := range u.Adj[current] {
			if _, visited := result[neighbor]; !visited {
				result[neighbor] = dist + 1
				queue = append(queue, neighbor)
			}
		}
	}
	return result
}

// ShortestPath returns the shortest jump count between origin and dest using Dijkstra.
// Returns -1 if no path exists.
func (u *Universe) ShortestPath(origin, dest int32) int {
	if origin == dest {
		return 0
	}

	dist := make(map[int32]int)
	dist[origin] = 0

	pq := &priorityQueue{{systemID: origin, dist: 0}}
	heap.Init(pq)

	for pq.Len() > 0 {
		item := heap.Pop(pq).(pqItem)
		if item.systemID == dest {
			return item.dist
		}
		if d, ok := dist[item.systemID]; ok && item.dist > d {
			continue
		}
		for _, neighbor := range u.Adj[item.systemID] {
			nd := item.dist + 1
			if d, ok := dist[neighbor]; !ok || nd < d {
				dist[neighbor] = nd
				heap.Push(pq, pqItem{systemID: neighbor, dist: nd})
			}
		}
	}
	return -1
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

// Priority queue for Dijkstra
type pqItem struct {
	systemID int32
	dist     int
}

type priorityQueue []pqItem

func (pq priorityQueue) Len() int            { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool   { return pq[i].dist < pq[j].dist }
func (pq priorityQueue) Swap(i, j int)        { pq[i], pq[j] = pq[j], pq[i] }
func (pq *priorityQueue) Push(x interface{}) { *pq = append(*pq, x.(pqItem)) }
func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}
