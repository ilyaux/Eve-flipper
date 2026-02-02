package graph

import "testing"

func makeTestUniverse() *Universe {
	// Simple graph: 1-2-3-4, 1-3 (shortcut)
	//   1 -- 2 -- 3 -- 4
	//   |         |
	//   +---------+
	u := &Universe{
		Adj: map[int32][]int32{
			1: {2, 3},
			2: {1, 3},
			3: {2, 1, 4},
			4: {3},
		},
		SystemRegion: map[int32]int32{
			1: 10, 2: 10, 3: 10, 4: 10,
		},
	}
	return u
}

func TestShortestPath_SameSystem(t *testing.T) {
	u := makeTestUniverse()
	if d := u.ShortestPath(1, 1); d != 0 {
		t.Errorf("ShortestPath(1,1) = %d, want 0", d)
	}
}

func TestShortestPath_Adjacent(t *testing.T) {
	u := makeTestUniverse()
	if d := u.ShortestPath(1, 2); d != 1 {
		t.Errorf("ShortestPath(1,2) = %d, want 1", d)
	}
}

func TestShortestPath_Shortcut(t *testing.T) {
	u := makeTestUniverse()
	// 1->3 direct = 1 jump (shortcut exists)
	if d := u.ShortestPath(1, 3); d != 1 {
		t.Errorf("ShortestPath(1,3) = %d, want 1", d)
	}
}

func TestShortestPath_MultiHop(t *testing.T) {
	u := makeTestUniverse()
	// 1->4: best = 1->3->4 = 2 jumps
	if d := u.ShortestPath(1, 4); d != 2 {
		t.Errorf("ShortestPath(1,4) = %d, want 2", d)
	}
}

func TestShortestPath_Unreachable(t *testing.T) {
	u := makeTestUniverse()
	// System 99 doesn't exist
	if d := u.ShortestPath(1, 99); d != -1 {
		t.Errorf("ShortestPath(1,99) = %d, want -1", d)
	}
}

func TestSystemsWithinRadius(t *testing.T) {
	u := makeTestUniverse()

	r0 := u.SystemsWithinRadius(1, 0)
	if len(r0) != 1 || r0[1] != 0 {
		t.Errorf("radius 0: got %v, want {1:0}", r0)
	}

	r1 := u.SystemsWithinRadius(1, 1)
	if len(r1) != 3 { // 1, 2, 3
		t.Errorf("radius 1: got %d systems, want 3", len(r1))
	}

	r2 := u.SystemsWithinRadius(1, 2)
	if len(r2) != 4 { // all
		t.Errorf("radius 2: got %d systems, want 4", len(r2))
	}
}

func TestRegionsInSet(t *testing.T) {
	u := makeTestUniverse()
	systems := map[int32]int{1: 0, 2: 1}
	regions := u.RegionsInSet(systems)
	if len(regions) != 1 || !regions[10] {
		t.Errorf("RegionsInSet: got %v, want {10:true}", regions)
	}
}
