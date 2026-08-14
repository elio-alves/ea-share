//go:build windows

package inject

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"kbs/internal/keys"
)

const (
	inputMouseType    = 0
	inputKeyboardType = 1

	mouseEventFMove       = 0x0001
	mouseEventFLeftDown   = 0x0002
	mouseEventFLeftUp     = 0x0004
	mouseEventFRightDown  = 0x0008
	mouseEventFRightUp    = 0x0010
	mouseEventFMiddleDown = 0x0020
	mouseEventFMiddleUp   = 0x0040
	mouseEventFWheel      = 0x0800

	keyEventFExtendedKey = 0x0001
	keyEventFKeyUp       = 0x0002
)

var (
	user32        = windows.NewLazySystemDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

// keybdInput/mouseInputStruct mirror the Win32 KEYBDINPUT/MOUSEINPUT
// structures. inputKB/inputMouseMsg mirror the tagged-union INPUT
// structure; both are padded to the same total size (matching
// sizeof(INPUT) on amd64/arm64) since SendInput validates cbSize against
// the real union size regardless of which variant is in use.
type keybdInput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type mouseInputStruct struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type inputKB struct {
	Type    uint32
	Ki      keybdInput
	Padding uint64
}

type inputMouseMsg struct {
	Type uint32
	Mi   mouseInputStruct
}

// extendedKeys are virtual-key codes that require KEYEVENTF_EXTENDEDKEY to
// behave correctly (arrows, nav cluster, right-side modifiers, Windows keys).
var extendedKeys = map[uint32]bool{
	0x25: true, 0x26: true, 0x27: true, 0x28: true, // left, up, right, down
	0x24: true, 0x23: true, 0x21: true, 0x22: true, // home, end, pgup, pgdn
	0x2D: true, 0x2E: true, // insert, delete
	0xA3: true, 0xA5: true, // right control, right alt
	0x5B: true, 0x5C: true, // left win, right win
}

type windowsInjector struct{}

// New returns an Injector backed by the Win32 SendInput API.
func New() (Injector, error) { return windowsInjector{}, nil }

func (windowsInjector) Key(key keys.Name, down bool) error {
	vk, ok := keys.NameToVK[key]
	if !ok {
		return fmt.Errorf("inject: unmapped key %q", key)
	}
	var flags uint32
	if !down {
		flags |= keyEventFKeyUp
	}
	if extendedKeys[vk] {
		flags |= keyEventFExtendedKey
	}
	in := inputKB{
		Type: inputKeyboardType,
		Ki: keybdInput{
			WVk:     uint16(vk),
			DwFlags: flags,
		},
	}
	return sendInput(unsafe.Pointer(&in), unsafe.Sizeof(in))
}

func (windowsInjector) MouseMove(dx, dy int32) error {
	in := inputMouseMsg{
		Type: inputMouseType,
		Mi: mouseInputStruct{
			Dx: dx, Dy: dy,
			DwFlags: mouseEventFMove,
		},
	}
	return sendInput(unsafe.Pointer(&in), unsafe.Sizeof(in))
}

func (windowsInjector) MouseButton(button MouseButton, down bool) error {
	var flag uint32
	switch button {
	case ButtonLeft:
		if down {
			flag = mouseEventFLeftDown
		} else {
			flag = mouseEventFLeftUp
		}
	case ButtonRight:
		if down {
			flag = mouseEventFRightDown
		} else {
			flag = mouseEventFRightUp
		}
	case ButtonMiddle:
		if down {
			flag = mouseEventFMiddleDown
		} else {
			flag = mouseEventFMiddleUp
		}
	default:
		return fmt.Errorf("inject: unknown button %q", button)
	}
	in := inputMouseMsg{
		Type: inputMouseType,
		Mi:   mouseInputStruct{DwFlags: flag},
	}
	return sendInput(unsafe.Pointer(&in), unsafe.Sizeof(in))
}

func (windowsInjector) MouseWheel(amount int32) error {
	in := inputMouseMsg{
		Type: inputMouseType,
		Mi: mouseInputStruct{
			MouseData: uint32(amount * 120),
			DwFlags:   mouseEventFWheel,
		},
	}
	return sendInput(unsafe.Pointer(&in), unsafe.Sizeof(in))
}

func (windowsInjector) Close() error { return nil }

func sendInput(p unsafe.Pointer, size uintptr) error {
	ret, _, err := procSendInput.Call(1, uintptr(p), size)
	if ret == 0 {
		return fmt.Errorf("inject: SendInput failed: %w", err)
	}
	return nil
}
