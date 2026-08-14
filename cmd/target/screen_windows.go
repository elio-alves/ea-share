//go:build windows

package main

import "kbs/internal/screen"

func ownScreenBounds() (w, h int32) {
	b := screen.GetBounds()
	return b.W, b.H
}

func warpCursor(x, y int32) error {
	return screen.SetCursorPos(x, y)
}
