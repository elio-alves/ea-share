//go:build windows

package keys

// Windows virtual-key codes, see:
// https://learn.microsoft.com/windows/win32/inputdev/virtual-key-codes
const (
	vkBack      = 0x08
	vkTab       = 0x09
	vkReturn    = 0x0D
	vkShift     = 0x10
	vkControl   = 0x11
	vkMenu      = 0x12
	vkEscape    = 0x1B
	vkSpace     = 0x20
	vkPrior     = 0x21
	vkNext      = 0x22
	vkEnd       = 0x23
	vkHome      = 0x24
	vkLeft      = 0x25
	vkUp        = 0x26
	vkRight     = 0x27
	vkDown      = 0x28
	vkInsert    = 0x2D
	vkDelete    = 0x2E
	vk0         = 0x30
	vkA         = 0x41
	vkF1        = 0x70
	vkCapital   = 0x14
	vkLShift    = 0xA0
	vkRShift    = 0xA1
	vkLControl  = 0xA2
	vkRControl  = 0xA3
	vkLMenu     = 0xA4
	vkRMenu     = 0xA5
	vkLWin      = 0x5B
	vkRWin      = 0x5C
	vkOEM1      = 0xBA // ;:
	vkOEMPlus   = 0xBB // =+
	vkOEMComma  = 0xBC // ,<
	vkOEMMinus  = 0xBD // -_
	vkOEMPeriod = 0xBE // .>
	vkOEM2      = 0xBF // /?
	vkOEM3      = 0xC0 // `~
	vkOEM4      = 0xDB // [{
	vkOEM5      = 0xDC // \|
	vkOEM6      = 0xDD // ]}
	vkOEM7      = 0xDE // '"
)

// VKToName maps a Windows virtual-key code to its platform-independent
// name. Generated once at init from NameToVK.
var VKToName map[uint32]Name

// NameToVK maps a platform-independent key name to its Windows virtual-key
// code.
var NameToVK = map[Name]uint32{
	A: vkA, B: vkA + 1, C: vkA + 2, D: vkA + 3, E: vkA + 4, F: vkA + 5,
	G: vkA + 6, H: vkA + 7, I: vkA + 8, J: vkA + 9, K: vkA + 10, L: vkA + 11,
	M: vkA + 12, N: vkA + 13, O: vkA + 14, P: vkA + 15, Q: vkA + 16, R: vkA + 17,
	S: vkA + 18, T: vkA + 19, U: vkA + 20, V: vkA + 21, W: vkA + 22, X: vkA + 23,
	Y: vkA + 24, Z: vkA + 25,

	N0: vk0, N1: vk0 + 1, N2: vk0 + 2, N3: vk0 + 3, N4: vk0 + 4,
	N5: vk0 + 5, N6: vk0 + 6, N7: vk0 + 7, N8: vk0 + 8, N9: vk0 + 9,

	F1: vkF1, F2: vkF1 + 1, F3: vkF1 + 2, F4: vkF1 + 3, F5: vkF1 + 4, F6: vkF1 + 5,
	F7: vkF1 + 6, F8: vkF1 + 7, F9: vkF1 + 8, F10: vkF1 + 9, F11: vkF1 + 10, F12: vkF1 + 11,

	Enter:      vkReturn,
	Escape:     vkEscape,
	Space:      vkSpace,
	Tab:        vkTab,
	Backspace:  vkBack,
	Delete:     vkDelete,
	Insert:     vkInsert,
	Home:       vkHome,
	End:        vkEnd,
	PageUp:     vkPrior,
	PageDown:   vkNext,
	CapsLock:   vkCapital,
	ArrowLeft:  vkLeft,
	ArrowRight: vkRight,
	ArrowUp:    vkUp,
	ArrowDown:  vkDown,

	ShiftLeft:    vkLShift,
	ShiftRight:   vkRShift,
	ControlLeft:  vkLControl,
	ControlRight: vkRControl,
	AltLeft:      vkLMenu,
	AltRight:     vkRMenu,
	MetaLeft:     vkLWin,
	MetaRight:    vkRWin,

	Minus:        vkOEMMinus,
	Equal:        vkOEMPlus,
	LeftBracket:  vkOEM4,
	RightBracket: vkOEM6,
	Backslash:    vkOEM5,
	Semicolon:    vkOEM1,
	Quote:        vkOEM7,
	Comma:        vkOEMComma,
	Period:       vkOEMPeriod,
	Slash:        vkOEM2,
	Grave:        vkOEM3,
}

func init() {
	VKToName = make(map[uint32]Name, len(NameToVK)+3)
	for name, vk := range NameToVK {
		VKToName[vk] = name
	}
	// Low-level keyboard hooks normally report the specific left/right VK
	// (handled above), but map the generic codes too as a fallback.
	VKToName[vkShift] = ShiftLeft
	VKToName[vkControl] = ControlLeft
	VKToName[vkMenu] = AltLeft
}
