//go:build windows

package capture

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"kbs/internal/keys"
	"kbs/internal/screen"
)

const (
	whKeyboardLL = 13
	whMouseLL    = 14

	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105

	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmRButtonDown = 0x0204
	wmRButtonUp   = 0x0205
	wmMButtonDown = 0x0207
	wmMButtonUp   = 0x0208
	wmMouseWheel  = 0x020A

	wmQuit  = 0x0012
	wmInput = 0x00FF

	ridevInputSink    = 0x00000100
	ridInput          = 0x10000003
	rimTypeMouse      = 0
	mouseMoveAbsolute = 0x01

	hwndMessageOnly = ^uintptr(2) // HWND_MESSAGE, i.e. (HWND)-3
)

var (
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookExW       = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx          = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx     = user32.NewProc("UnhookWindowsHookEx")
	procGetMessageW             = user32.NewProc("GetMessageW")
	procTranslateMessage        = user32.NewProc("TranslateMessage")
	procDispatchMessageW        = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW      = user32.NewProc("PostThreadMessageW")
	procRegisterClassExW        = user32.NewProc("RegisterClassExW")
	procUnregisterClassW        = user32.NewProc("UnregisterClassW")
	procCreateWindowExW         = user32.NewProc("CreateWindowExW")
	procDestroyWindow           = user32.NewProc("DestroyWindow")
	procDefWindowProcW          = user32.NewProc("DefWindowProcW")
	procRegisterRawInputDevices = user32.NewProc("RegisterRawInputDevices")
	procGetRawInputData         = user32.NewProc("GetRawInputData")

	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

type point struct{ X, Y int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type kbdllhookstruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type msllhookstruct struct {
	Pt          point
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// wndClassExW mirrors WNDCLASSEXW, used to register the hidden window that
// receives WM_INPUT messages.
type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

// rawInputDevice mirrors RAWINPUTDEVICE, passed to RegisterRawInputDevices.
type rawInputDevice struct {
	usUsagePage uint16
	usUsage     uint16
	dwFlags     uint32
	hwndTarget  uintptr
}

// rawInputHeader mirrors RAWINPUTHEADER.
type rawInputHeader struct {
	dwType  uint32
	dwSize  uint32
	hDevice uintptr
	wParam  uintptr
}

// rawMouse mirrors RAWMOUSE; only the fields needed for relative-motion
// deltas are named (the button-state union is read as a raw uint32).
type rawMouse struct {
	usFlags      uint16
	_            uint16
	ulButtons    uint32
	ulRawButtons uint32
	lLastX       int32
	lLastY       int32
	ulExtraInfo  uint32
}

// rawInputMouse mirrors RAWINPUT for a RIM_TYPEMOUSE record (header plus
// the mouse variant of the data union).
type rawInputMouse struct {
	header rawInputHeader
	mouse  rawMouse
}

// windowsSource is a global-hook based capture.Source. Only one instance
// should run at a time (Windows low-level hooks are inherently
// process-wide).
type windowsSource struct {
	events   chan Event
	threadID uint32

	// edge is empty for a plain (legacy) Source: input is always
	// forwarded and never suppressed. When set, the source starts
	// disengaged (watching only, no forwarding, no suppression) and
	// switches to engaged (forwarding + suppressing) once the cursor
	// crosses this edge; see EdgeAware.
	edge    Edge
	bounds  screen.Bounds
	engaged atomic.Bool

	hwnd uintptr // hidden message-only window used to receive WM_INPUT

	// ctrlHeld/altHeld/hotkeyVDown track modifier chord state for the
	// Ctrl+Alt+V clipboard-transfer hotkey. Only ever touched from
	// keyboardProc, which runs on a single (locked) OS thread, so no
	// synchronization is needed.
	ctrlHeld, altHeld, hotkeyVDown bool
}

// New returns a capture.Source backed by Windows low-level keyboard/mouse
// hooks. It always forwards input and never suppresses it locally.
func New() Source {
	return &windowsSource{}
}

// NewEdgeAware returns a capture.EdgeAware backed by the same hooks as
// New, but starting disengaged: local input passes through untouched and
// isn't forwarded until the cursor crosses edge, at which point local
// input is suppressed and forwarded until Disengage is called.
func NewEdgeAware(edge Edge) (EdgeAware, error) {
	return &windowsSource{edge: edge, bounds: screen.GetBounds()}, nil
}

// Disengage stops suppressing/forwarding local input and warps the cursor
// to (warpX, warpY), typically just inside the edge the controller
// originally exited through.
func (s *windowsSource) Disengage(warpX, warpY int32) error {
	s.engaged.Store(false)
	return screen.SetCursorPos(warpX, warpY)
}

func (s *windowsSource) Start() (<-chan Event, error) {
	s.events = make(chan Event, 2048)
	errCh := make(chan error, 1)
	readyCh := make(chan struct{})
	go s.run(errCh, readyCh)
	select {
	case err := <-errCh:
		return nil, err
	case <-readyCh:
	}
	return s.events, nil
}

func (s *windowsSource) Stop() error {
	if s.threadID != 0 {
		procPostThreadMessageW.Call(uintptr(s.threadID), wmQuit, 0, 0)
	}
	return nil
}

func (s *windowsSource) run(errCh chan error, readyCh chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	s.threadID = windows.GetCurrentThreadId()

	hwnd, unregister, err := s.createRawInputWindow()
	if err != nil {
		errCh <- err
		return
	}
	s.hwnd = hwnd
	defer unregister()

	kbCallback := syscall.NewCallback(s.keyboardProc)
	mouseCallback := syscall.NewCallback(s.mouseProc)

	kbHook, _, err := procSetWindowsHookExW.Call(uintptr(whKeyboardLL), kbCallback, 0, 0)
	if kbHook == 0 {
		errCh <- fmt.Errorf("capture: SetWindowsHookExW(keyboard): %w", err)
		return
	}
	defer procUnhookWindowsHookEx.Call(kbHook)

	mouseHook, _, err := procSetWindowsHookExW.Call(uintptr(whMouseLL), mouseCallback, 0, 0)
	if mouseHook == 0 {
		errCh <- fmt.Errorf("capture: SetWindowsHookExW(mouse): %w", err)
		return
	}
	defer procUnhookWindowsHookEx.Call(mouseHook)

	close(readyCh)

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	close(s.events)
}

// createRawInputWindow creates a hidden message-only window and registers
// it to receive WM_INPUT for the mouse, with RIDEV_INPUTSINK so deltas
// keep arriving even while another window has focus. Relative mouse
// deltas are read from here (RAWMOUSE.lLastX/lLastY) instead of from the
// WH_MOUSE_LL hook's cursor position, because that position stops
// advancing while suppressed (see mouseProc) and diffing it across a
// suppressed span produces garbage deltas.
func (s *windowsSource) createRawInputWindow() (hwnd uintptr, unregister func(), err error) {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, err := syscall.UTF16PtrFromString("kbs_capture_raw_input")
	if err != nil {
		return 0, nil, fmt.Errorf("capture: class name: %w", err)
	}

	wndProcCallback := syscall.NewCallback(s.wndProc)
	class := wndClassExW{
		lpfnWndProc:   wndProcCallback,
		hInstance:     syscall.Handle(hInstance),
		lpszClassName: className,
	}
	class.cbSize = uint32(unsafe.Sizeof(class))

	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return 0, nil, fmt.Errorf("capture: RegisterClassExW: %w", err)
	}

	hwnd, _, err = procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, 0,
		0, 0, 0, 0,
		hwndMessageOnly, 0, hInstance, 0,
	)
	if hwnd == 0 {
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)
		return 0, nil, fmt.Errorf("capture: CreateWindowExW: %w", err)
	}

	device := rawInputDevice{
		usUsagePage: 0x01, // generic desktop controls
		usUsage:     0x02, // mouse
		dwFlags:     ridevInputSink,
		hwndTarget:  hwnd,
	}
	ret, _, err := procRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&device)), 1, unsafe.Sizeof(device))
	if ret == 0 {
		procDestroyWindow.Call(hwnd)
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)
		return 0, nil, fmt.Errorf("capture: RegisterRawInputDevices: %w", err)
	}

	unregister = func() {
		procDestroyWindow.Call(hwnd)
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)
	}
	return hwnd, unregister, nil
}

// wndProc is the WNDPROC for the hidden raw-input window: it only cares
// about WM_INPUT and defers everything else to DefWindowProcW.
func (s *windowsSource) wndProc(hwnd uintptr, uMsg uint32, wParam, lParam uintptr) uintptr {
	if uMsg == wmInput {
		s.handleRawInput(lParam)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(uMsg), wParam, lParam)
	return ret
}

// handleRawInput reads a WM_INPUT payload and, for relative-motion mouse
// data, emits a MouseMoveEvent with the true HID delta - unaffected by
// cursor suppression, screen-edge clamping, or pointer ballistics.
func (s *windowsSource) handleRawInput(hRawInput uintptr) {
	var buf rawInputMouse
	size := uint32(unsafe.Sizeof(buf))
	ret, _, _ := procGetRawInputData.Call(
		hRawInput, ridInput,
		uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&size)),
		unsafe.Sizeof(rawInputHeader{}),
	)
	if int32(ret) < 0 {
		return
	}
	if buf.header.dwType != rimTypeMouse {
		return
	}
	if buf.mouse.usFlags&mouseMoveAbsolute != 0 {
		return // absolute-positioning device (e.g. RDP session); no usable relative delta
	}
	dx, dy := buf.mouse.lLastX, buf.mouse.lLastY
	if dx == 0 && dy == 0 {
		return
	}
	if forward, _ := s.forwardAndSuppress(); forward {
		s.sendEvent(Event{Kind: MouseMoveEvent, DX: dx, DY: dy})
	}
}

func (s *windowsSource) sendEvent(e Event) {
	select {
	case s.events <- e:
	default:
		// Drop the event rather than block the hook thread; a stalled
		// low-level hook is forcibly unhooked by Windows after a short
		// timeout, which would silently kill capture entirely.
	}
}

// LLKHF_INJECTED/LLMHF_INJECTED (and their LOWER_IL_INJECTED counterparts)
// mark events synthesized by SendInput rather than typed/moved by a human.
// Capture ignores these so that running a target's injector on the same
// machine as a controller's capture (e.g. for local testing) doesn't loop
// injected input back into another outgoing event.
const (
	llkhfInjected        = 0x00000010
	llkhfLowerILInjected = 0x00000002
	llmhfInjected        = 0x00000001
	llmhfLowerILInjected = 0x00000002
)

// forwardAndSuppress reports whether the current event should be
// forwarded (sent on the events channel) and suppressed (kept from
// reaching the rest of the system). In legacy mode (edge == "") it always
// forwards and never suppresses; in edge-aware mode both mirror whether
// the source is currently engaged.
func (s *windowsSource) forwardAndSuppress() (forward, suppress bool) {
	if s.edge == "" {
		return true, false
	}
	engaged := s.engaged.Load()
	return engaged, engaged
}

// keyboardProc matches the WH_KEYBOARD_LL callback signature: LRESULT
// CALLBACK(int nCode, WPARAM wParam, LPARAM lParam).
func (s *windowsSource) keyboardProc(nCode int32, wParam, lParam uintptr) uintptr {
	suppress := false
	if nCode >= 0 {
		kb := (*kbdllhookstruct)(unsafe.Pointer(lParam))
		injected := kb.Flags&(llkhfInjected|llkhfLowerILInjected) != 0
		if !injected {
			down := uint32(wParam) == wmKeyDown || uint32(wParam) == wmSysKeyDown
			name, known := keys.VKToName[kb.VkCode]
			if known {
				switch name {
				case keys.ControlLeft, keys.ControlRight:
					s.ctrlHeld = down
				case keys.AltLeft, keys.AltRight:
					s.altHeld = down
				}
			}

			switch {
			case known && name == keys.V && down && s.ctrlHeld && s.altHeld:
				// Ctrl+Alt+V: the clipboard-transfer hotkey. Detected and
				// suppressed unconditionally - even while disengaged,
				// unlike every other key - and never forwarded as a
				// literal keystroke; the matching key-up is swallowed
				// too so it doesn't leak through on its own. V (unlike
				// Ctrl/Alt) auto-repeats for as long as it's held, so
				// only fire once per physical press - repeats would spawn
				// overlapping paste-injection goroutines that race each
				// other's synthetic key up/down calls and can leave a
				// modifier stuck "held" afterwards.
				suppress = true
				if !s.hotkeyVDown {
					s.hotkeyVDown = true
					s.sendEvent(Event{Kind: HotkeyPasteEvent})
				}
			case known && name == keys.V && !down && s.hotkeyVDown:
				s.hotkeyVDown = false
				suppress = true
			default:
				var forward bool
				forward, suppress = s.forwardAndSuppress()
				if forward && known {
					s.sendEvent(Event{Kind: KeyEvent, Key: name, Down: down})
				}
			}
		}
	}
	if suppress {
		return 1
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

// edgeCrossed reports whether pt has reached edge of bounds, and if so how
// far along that edge (0.0-1.0).
func edgeCrossed(edge Edge, b screen.Bounds, pt point) (relPos float64, crossed bool) {
	switch edge {
	case EdgeLeft:
		if pt.X <= b.X {
			return clamp01(float64(pt.Y-b.Y) / float64(b.H)), true
		}
	case EdgeRight:
		if pt.X >= b.X+b.W-1 {
			return clamp01(float64(pt.Y-b.Y) / float64(b.H)), true
		}
	case EdgeTop:
		if pt.Y <= b.Y {
			return clamp01(float64(pt.X-b.X) / float64(b.W)), true
		}
	case EdgeBottom:
		if pt.Y >= b.Y+b.H-1 {
			return clamp01(float64(pt.X-b.X) / float64(b.W)), true
		}
	}
	return 0, false
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

// mouseProc matches the WH_MOUSE_LL callback signature.
func (s *windowsSource) mouseProc(nCode int32, wParam, lParam uintptr) uintptr {
	suppress := false
	if nCode >= 0 {
		ms := (*msllhookstruct)(unsafe.Pointer(lParam))
		injected := ms.Flags&(llmhfInjected|llmhfLowerILInjected) != 0
		if !injected {
			switch uint32(wParam) {
			case wmMouseMove:
				// Deltas are captured separately via Raw Input (see
				// handleRawInput); this case only detects edge-crossing
				// (which needs the absolute cursor position) and decides
				// suppression.
				_, doSuppress := s.forwardAndSuppress()
				if s.edge != "" && !doSuppress {
					if relPos, crossed := edgeCrossed(s.edge, s.bounds, ms.Pt); crossed {
						s.engaged.Store(true)
						doSuppress = true // swallow the move that triggered the crossing
						s.sendEvent(Event{Kind: EdgeCrossedEvent, RelPos: relPos})
					}
				}
				suppress = doSuppress
			case wmLButtonDown:
				var forward bool
				forward, suppress = s.forwardAndSuppress()
				if forward {
					s.sendEvent(Event{Kind: MouseButtonEvent, Button: ButtonLeft, Down: true})
				}
			case wmLButtonUp:
				var forward bool
				forward, suppress = s.forwardAndSuppress()
				if forward {
					s.sendEvent(Event{Kind: MouseButtonEvent, Button: ButtonLeft, Down: false})
				}
			case wmRButtonDown:
				var forward bool
				forward, suppress = s.forwardAndSuppress()
				if forward {
					s.sendEvent(Event{Kind: MouseButtonEvent, Button: ButtonRight, Down: true})
				}
			case wmRButtonUp:
				var forward bool
				forward, suppress = s.forwardAndSuppress()
				if forward {
					s.sendEvent(Event{Kind: MouseButtonEvent, Button: ButtonRight, Down: false})
				}
			case wmMButtonDown:
				var forward bool
				forward, suppress = s.forwardAndSuppress()
				if forward {
					s.sendEvent(Event{Kind: MouseButtonEvent, Button: ButtonMiddle, Down: true})
				}
			case wmMButtonUp:
				var forward bool
				forward, suppress = s.forwardAndSuppress()
				if forward {
					s.sendEvent(Event{Kind: MouseButtonEvent, Button: ButtonMiddle, Down: false})
				}
			case wmMouseWheel:
				var forward bool
				forward, suppress = s.forwardAndSuppress()
				if forward {
					hiword := int16(uint16(ms.MouseData >> 16))
					s.sendEvent(Event{Kind: MouseWheelEvent, Amount: int32(hiword) / 120})
				}
			}
		}
	}
	if suppress {
		return 1
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}
