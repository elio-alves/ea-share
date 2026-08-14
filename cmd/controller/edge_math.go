package main

// The pure geometry behind edge-triggered switching (see edge_windows.go),
// deliberately kept free of any OS/build-tag dependency so it can be unit
// tested on any platform, including a Linux CI runner that can never build
// or run the Windows-only capture/inject code itself. This is the exact
// math responsible for two real bugs during development (see
// docs/known-issues.md) - keep it covered.

import "kbs/internal/protocol"

// entryPosition is where a cursor crossing into entryEdge should start,
// relPos fraction of the way along that edge.
func entryPosition(entryEdge protocol.Edge, relPos float64, w, h int32) (x, y int32) {
	switch entryEdge {
	case protocol.EdgeLeft:
		return 0, int32(relPos * float64(h))
	case protocol.EdgeRight:
		return w - 1, int32(relPos * float64(h))
	case protocol.EdgeTop:
		return int32(relPos * float64(w)), 0
	case protocol.EdgeBottom:
		return int32(relPos * float64(w)), h - 1
	default:
		return 0, 0
	}
}

// pushesPast reports whether (x, y) has been pushed beyond edge of a w x h
// screen.
func pushesPast(edge protocol.Edge, x, y, w, h int32) bool {
	switch edge {
	case protocol.EdgeLeft:
		return x < 0
	case protocol.EdgeRight:
		return x > w-1
	case protocol.EdgeTop:
		return y < 0
	case protocol.EdgeBottom:
		return y > h-1
	default:
		return false
	}
}

// hasMovedAway reports whether (x, y), already clamped inside a w x h
// screen, is strictly off the given edge.
func hasMovedAway(edge protocol.Edge, x, y, w, h int32) bool {
	switch edge {
	case protocol.EdgeLeft:
		return x > 0
	case protocol.EdgeRight:
		return x < w-1
	case protocol.EdgeTop:
		return y > 0
	case protocol.EdgeBottom:
		return y < h-1
	default:
		return false
	}
}

// releaseRelPos computes how far along edge the (clamped) release point
// is.
func releaseRelPos(edge protocol.Edge, x, y, w, h int32) float64 {
	switch edge {
	case protocol.EdgeLeft, protocol.EdgeRight:
		return clamp01(float64(y) / float64(h))
	default:
		return clamp01(float64(x) / float64(w))
	}
}

// controllerWarpPosition is where the controller's own cursor should land,
// just inside triggerEdge, when control returns to it. bx/by/bw/bh are the
// controller's own screen bounds (screen.Bounds's fields, passed
// individually so this file doesn't need to import the Windows-only
// internal/screen package).
func controllerWarpPosition(triggerEdge protocol.Edge, relPos float64, bx, by, bw, bh int32) (x, y int32) {
	switch triggerEdge {
	case protocol.EdgeLeft:
		return bx + 1, by + int32(relPos*float64(bh))
	case protocol.EdgeRight:
		return bx + bw - 2, by + int32(relPos*float64(bh))
	case protocol.EdgeTop:
		return bx + int32(relPos*float64(bw)), by + 1
	case protocol.EdgeBottom:
		return bx + int32(relPos*float64(bw)), by + bh - 2
	default:
		return bx, by
	}
}

func clampI(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
