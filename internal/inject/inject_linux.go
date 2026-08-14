//go:build linux

package inject

import (
	"fmt"

	"github.com/bendahl/uinput"

	"kbs/internal/keys"
)

// virtualDeviceMarker must match the name substring checked by the Linux
// capture backend (capture_linux.go) so it can ignore these synthetic
// devices when both controller and target run on the same machine.
const virtualDeviceMarker = "kbs-virtual-input"

type linuxInjector struct {
	kb    uinput.Keyboard
	mouse uinput.Mouse
}

// New creates virtual keyboard and mouse devices via uinput and returns an
// Injector backed by them. Requires read/write access to /dev/uinput
// (typically root, or membership in a group granted access via udev rules).
func New() (Injector, error) {
	kb, err := uinput.CreateKeyboard("/dev/uinput", []byte(virtualDeviceMarker+"-kbd"))
	if err != nil {
		return nil, fmt.Errorf("inject: creating virtual keyboard: %w", err)
	}
	mouse, err := uinput.CreateMouse("/dev/uinput", []byte(virtualDeviceMarker+"-mouse"))
	if err != nil {
		kb.Close()
		return nil, fmt.Errorf("inject: creating virtual mouse: %w", err)
	}
	return &linuxInjector{kb: kb, mouse: mouse}, nil
}

func (i *linuxInjector) Key(key keys.Name, down bool) error {
	code, ok := keys.NameToKeycode[key]
	if !ok {
		return fmt.Errorf("inject: unmapped key %q", key)
	}
	if down {
		return i.kb.KeyDown(code)
	}
	return i.kb.KeyUp(code)
}

func (i *linuxInjector) MouseMove(dx, dy int32) error {
	return i.mouse.Move(dx, dy)
}

func (i *linuxInjector) MouseButton(button MouseButton, down bool) error {
	switch button {
	case ButtonLeft:
		if down {
			return i.mouse.LeftPress()
		}
		return i.mouse.LeftRelease()
	case ButtonRight:
		if down {
			return i.mouse.RightPress()
		}
		return i.mouse.RightRelease()
	case ButtonMiddle:
		if down {
			return i.mouse.MiddlePress()
		}
		return i.mouse.MiddleRelease()
	default:
		return fmt.Errorf("inject: unknown button %q", button)
	}
}

func (i *linuxInjector) MouseWheel(amount int32) error {
	return i.mouse.Wheel(false, amount)
}

func (i *linuxInjector) Close() error {
	err1 := i.kb.Close()
	err2 := i.mouse.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
