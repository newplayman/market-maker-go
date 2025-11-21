package strategy

import "testing"

func TestBuildDynamicGrid(t *testing.T) {
	grid := BuildDynamicGrid(100, 0.5, 3, 1)
	if len(grid) != 6 {
		t.Fatalf("expected 6 levels, got %d", len(grid))
	}
	// 应上下对称
	if grid[0].Price >= 100 || grid[1].Price <= 100 {
		t.Fatalf("unexpected grid symmetry: %+v", grid[:2])
	}
}
