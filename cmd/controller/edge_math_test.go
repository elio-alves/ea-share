package main

import (
	"testing"

	"kbs/internal/protocol"
)

func TestEntryPosition(t *testing.T) {
	const w, h = 1920, 1080

	cases := []struct {
		edge         protocol.Edge
		relPos       float64
		wantX, wantY int32
	}{
		{protocol.EdgeLeft, 0.5, 0, 540},
		{protocol.EdgeRight, 0.5, w - 1, 540},
		{protocol.EdgeTop, 0.5, 960, 0},
		{protocol.EdgeBottom, 0.5, 960, h - 1},
		{protocol.EdgeLeft, 0, 0, 0},
		{protocol.EdgeLeft, 1, 0, h},
	}
	for _, c := range cases {
		gotX, gotY := entryPosition(c.edge, c.relPos, w, h)
		if gotX != c.wantX || gotY != c.wantY {
			t.Errorf("entryPosition(%s, %v, %d, %d) = (%d, %d), want (%d, %d)",
				c.edge, c.relPos, w, h, gotX, gotY, c.wantX, c.wantY)
		}
	}
}

func TestPushesPast(t *testing.T) {
	const w, h = 1920, 1080

	cases := []struct {
		edge protocol.Edge
		x, y int32
		want bool
	}{
		{protocol.EdgeLeft, -1, 0, true},
		{protocol.EdgeLeft, 0, 0, false},
		{protocol.EdgeRight, w, 0, true},
		{protocol.EdgeRight, w - 1, 0, false},
		{protocol.EdgeTop, 0, -1, true},
		{protocol.EdgeTop, 0, 0, false},
		{protocol.EdgeBottom, 0, h, true},
		{protocol.EdgeBottom, 0, h - 1, false},
	}
	for _, c := range cases {
		if got := pushesPast(c.edge, c.x, c.y, w, h); got != c.want {
			t.Errorf("pushesPast(%s, %d, %d, %d, %d) = %v, want %v", c.edge, c.x, c.y, w, h, got, c.want)
		}
	}
}

func TestHasMovedAway(t *testing.T) {
	const w, h = 1920, 1080

	cases := []struct {
		edge protocol.Edge
		x, y int32
		want bool
	}{
		{protocol.EdgeLeft, 0, 0, false}, // still sitting right at the entry edge
		{protocol.EdgeLeft, 1, 0, true},
		{protocol.EdgeRight, w - 1, 0, false},
		{protocol.EdgeRight, w - 2, 0, true},
		{protocol.EdgeTop, 0, 0, false},
		{protocol.EdgeTop, 0, 1, true},
		{protocol.EdgeBottom, 0, h - 1, false},
		{protocol.EdgeBottom, 0, h - 2, true},
	}
	for _, c := range cases {
		if got := hasMovedAway(c.edge, c.x, c.y, w, h); got != c.want {
			t.Errorf("hasMovedAway(%s, %d, %d, %d, %d) = %v, want %v", c.edge, c.x, c.y, w, h, got, c.want)
		}
	}
}

func TestReleaseRelPos(t *testing.T) {
	const w, h = 1920, 1080

	cases := []struct {
		edge protocol.Edge
		x, y int32
		want float64
	}{
		{protocol.EdgeLeft, 0, 540, 0.5}, // left/right: position along the vertical axis
		{protocol.EdgeRight, w - 1, 270, 0.25},
		{protocol.EdgeTop, 960, 0, 0.5}, // top/bottom: position along the horizontal axis
		{protocol.EdgeBottom, 480, h - 1, 0.25},
		{protocol.EdgeLeft, 0, -100, 0}, // clamped into [0, 1]
		{protocol.EdgeLeft, 0, h + 100, 1},
	}
	for _, c := range cases {
		if got := releaseRelPos(c.edge, c.x, c.y, w, h); got != c.want {
			t.Errorf("releaseRelPos(%s, %d, %d, %d, %d) = %v, want %v", c.edge, c.x, c.y, w, h, got, c.want)
		}
	}
}

func TestControllerWarpPosition(t *testing.T) {
	const bx, by, bw, bh = 0, 0, 1920, 1080

	cases := []struct {
		edge         protocol.Edge
		relPos       float64
		wantX, wantY int32
	}{
		{protocol.EdgeLeft, 0.5, 1, 540},
		{protocol.EdgeRight, 0.5, bw - 2, 540},
		{protocol.EdgeTop, 0.5, 960, 1},
		{protocol.EdgeBottom, 0.5, 960, bh - 2},
	}
	for _, c := range cases {
		gotX, gotY := controllerWarpPosition(c.edge, c.relPos, bx, by, bw, bh)
		if gotX != c.wantX || gotY != c.wantY {
			t.Errorf("controllerWarpPosition(%s, %v, ...) = (%d, %d), want (%d, %d)",
				c.edge, c.relPos, gotX, gotY, c.wantX, c.wantY)
		}
		// The warp point must always land strictly inside the controller's
		// own screen, one pixel clear of the edge - landing exactly on it
		// (or outside it) would immediately re-trigger the crossing that
		// caused the disengage in the first place.
		if gotX < bx || gotX >= bx+bw || gotY < by || gotY >= by+bh {
			t.Errorf("controllerWarpPosition(%s, ...) = (%d, %d) lands outside the screen bounds", c.edge, gotX, gotY)
		}
	}
}

func TestClampI(t *testing.T) {
	cases := []struct{ v, lo, hi, want int32 }{
		{5, 0, 10, 5},
		{-1, 0, 10, 0},
		{11, 0, 10, 10},
		{0, 0, 10, 0},
		{10, 0, 10, 10},
	}
	for _, c := range cases {
		if got := clampI(c.v, c.lo, c.hi); got != c.want {
			t.Errorf("clampI(%d, %d, %d) = %d, want %d", c.v, c.lo, c.hi, got, c.want)
		}
	}
}

func TestClamp01(t *testing.T) {
	cases := []struct{ v, want float64 }{
		{0.5, 0.5},
		{-0.1, 0},
		{1.1, 1},
		{0, 0},
		{1, 1},
	}
	for _, c := range cases {
		if got := clamp01(c.v); got != c.want {
			t.Errorf("clamp01(%v) = %v, want %v", c.v, got, c.want)
		}
	}
}

// TestEngageDisengageRoundTrip exercises the exact sequence behind the two
// mouse bugs fixed during development (see docs/known-issues.md): engaging
// at one point along the entry edge, moving away from it, then pushing
// back out through that same edge must both (a) be recognized as a
// release and (b) compute a release position consistent with where the
// cursor actually crossed back out.
func TestEngageDisengageRoundTrip(t *testing.T) {
	const targetW, targetH = 1920, 1080
	edge := protocol.EdgeRight // target is to the right of the controller
	entryEdge := edge.Opposite()

	vx, vy := entryPosition(entryEdge, 0.5, targetW, targetH)
	if vx != 0 {
		t.Fatalf("entry position for EdgeLeft should sit on x=0, got x=%d", vx)
	}

	// Move away from the entry edge - must not look like a release yet.
	vx = vx + 50
	if pushesPast(entryEdge, vx, vy, targetW, targetH) {
		t.Fatalf("moving further into the screen should not register as pushing past the entry edge")
	}
	if !hasMovedAway(entryEdge, vx, vy, targetW, targetH) {
		t.Fatalf("expected hasMovedAway to be true after moving away from the entry edge")
	}

	// Push back out past the same edge - this is the release.
	vx = vx - 200 // overshoot past x=0
	if !pushesPast(entryEdge, vx, vy, targetW, targetH) {
		t.Fatalf("expected pushesPast to be true after pushing back past the entry edge")
	}
	relPos := releaseRelPos(entryEdge, clampI(vx, 0, targetW-1), clampI(vy, 0, targetH-1), targetW, targetH)
	if relPos != clamp01(float64(vy)/float64(targetH)) {
		t.Errorf("release relPos = %v, want the clamped vertical fraction", relPos)
	}
}
