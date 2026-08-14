// Package capture reads local keyboard/mouse input and emits it as
// platform-independent Events. Each supported OS provides its own backend
// (see capture_windows.go, capture_linux.go).
package capture

import "kbs/internal/keys"

type EventKind int

const (
	KeyEvent EventKind = iota
	MouseMoveEvent
	MouseButtonEvent
	MouseWheelEvent

	// EdgeCrossedEvent is only emitted by an edge-aware Source (see
	// EdgeAware): the cursor reached the configured screen edge. The
	// source has already started suppressing local input by the time
	// this is delivered.
	EdgeCrossedEvent

	// HotkeyPasteEvent fires when the user presses the clipboard-transfer
	// hotkey (Ctrl+Alt+V), regardless of engaged/disengaged state — the
	// caller decides the transfer direction from its own state. The
	// hotkey itself is always suppressed (never forwarded as a literal
	// key), on Windows.
	HotkeyPasteEvent
)

type MouseButton string

const (
	ButtonLeft   MouseButton = "left"
	ButtonRight  MouseButton = "right"
	ButtonMiddle MouseButton = "middle"
)

// Edge identifies one side of a screen.
type Edge string

const (
	EdgeLeft   Edge = "left"
	EdgeRight  Edge = "right"
	EdgeTop    Edge = "top"
	EdgeBottom Edge = "bottom"
)

// Event is a single captured input event. Only the fields relevant to Kind
// are populated.
type Event struct {
	Kind EventKind

	// KeyEvent
	Key  keys.Name
	Down bool

	// MouseMoveEvent: relative pixel deltas since the previous move.
	DX, DY int32

	// MouseButtonEvent
	Button MouseButton

	// MouseWheelEvent: positive = scroll up/away from the user.
	Amount int32

	// EdgeCrossedEvent: how far along the edge (0.0-1.0) the cursor was
	// when it crossed.
	RelPos float64
}

// Source captures local input and delivers Events on the returned channel
// until Stop is called or the underlying OS hook fails.
type Source interface {
	// Start begins capturing and returns a channel of events. The channel
	// is closed when capture stops.
	Start() (<-chan Event, error)
	// Stop releases the OS hook(s) and closes the event channel.
	Stop() error
}

// EdgeAware is implemented by capture sources that support edge-triggered
// switching (currently Windows only). While disengaged, local input passes
// through untouched and the source only watches for the cursor reaching
// the configured edge (delivered as an EdgeCrossedEvent). Once engaged,
// local mouse/keyboard input is suppressed (never reaches other
// applications) and every event is also delivered on the channel, for the
// caller to forward to a target. Disengage ends suppression and warps the
// local cursor back onto the screen.
type EdgeAware interface {
	Source
	Disengage(warpX, warpY int32) error
}
