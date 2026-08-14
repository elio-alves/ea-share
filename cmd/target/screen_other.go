//go:build !windows

package main

import "errors"

func ownScreenBounds() (w, h int32) {
	return 0, 0
}

func warpCursor(x, y int32) error {
	return errors.New("edge switching (cursor warp) is only supported on Windows right now")
}
