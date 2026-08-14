// Package inject synthesizes local keyboard/mouse input from
// platform-independent events, mirroring what a controller captured
// remotely. Each supported OS provides its own backend (see
// inject_windows.go, inject_linux.go).
package inject

import "kbs/internal/keys"

type MouseButton string

const (
	ButtonLeft   MouseButton = "left"
	ButtonRight  MouseButton = "right"
	ButtonMiddle MouseButton = "middle"
)

// Injector synthesizes local input events.
type Injector interface {
	// Key presses or releases a key identified by its platform-independent
	// name.
	Key(key keys.Name, down bool) error
	// MouseMove moves the cursor by a relative pixel offset.
	MouseMove(dx, dy int32) error
	// MouseButton presses or releases a mouse button.
	MouseButton(button MouseButton, down bool) error
	// MouseWheel scrolls vertically; positive amount scrolls up.
	MouseWheel(amount int32) error
	// Close releases any OS resources held by the injector.
	Close() error
}
