// Package screen reports display geometry and controls the cursor
// position, used for edge-triggered controller/target switching. Windows
// only for now (see internal/capture and internal/inject for the same
// scoping).
package screen

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
)

// Bounds is the bounding rectangle of the full virtual desktop (the union
// of all monitors), in pixels.
type Bounds struct {
	X, Y, W, H int32
}

// GetBounds returns the virtual desktop's bounding rectangle.
func GetBounds() Bounds {
	return Bounds{
		X: int32(getMetric(smXVirtualScreen)),
		Y: int32(getMetric(smYVirtualScreen)),
		W: int32(getMetric(smCXVirtualScreen)),
		H: int32(getMetric(smCYVirtualScreen)),
	}
}

func getMetric(index int) int32 {
	ret, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int32(ret)
}

type point struct{ X, Y int32 }

// GetCursorPos returns the current cursor position in screen coordinates.
func GetCursorPos() (x, y int32, err error) {
	var pt point
	ret, _, callErr := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	if ret == 0 {
		return 0, 0, fmt.Errorf("screen: GetCursorPos: %w", callErr)
	}
	return pt.X, pt.Y, nil
}

// SetCursorPos moves the cursor to the given screen coordinates.
func SetCursorPos(x, y int32) error {
	ret, _, err := procSetCursorPos.Call(uintptr(x), uintptr(y))
	if ret == 0 {
		return fmt.Errorf("screen: SetCursorPos: %w", err)
	}
	return nil
}
