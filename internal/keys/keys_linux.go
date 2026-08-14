//go:build linux

package keys

// Linux kernel input-event-codes (see linux/input-event-codes.h). Values
// match both github.com/gvalkov/golang-evdev and github.com/bendahl/uinput,
// so this single table serves both the capture and inject backends.
const (
	keyEsc = 1
	key1   = 2
	key2   = 3
	key3   = 4
	key4   = 5
	key5   = 6
	key6   = 7
	key7   = 8
	key8   = 9
	key9   = 10
	key0   = 11

	keyMinus     = 12
	keyEqual     = 13
	keyBackspace = 14
	keyTab       = 15

	keyQ = 16
	keyW = 17
	keyE = 18
	keyR = 19
	keyT = 20
	keyY = 21
	keyU = 22
	keyI = 23
	keyO = 24
	keyP = 25

	keyLeftbrace  = 26
	keyRightbrace = 27
	keyEnter      = 28
	keyLeftctrl   = 29

	keyA = 30
	keyS = 31
	keyD = 32
	keyF = 33
	keyG = 34
	keyH = 35
	keyJ = 36
	keyK = 37
	keyL = 38

	keySemicolon  = 39
	keyApostrophe = 40
	keyGrave      = 41
	keyLeftshift  = 42
	keyBackslash  = 43

	keyZ = 44
	keyX = 45
	keyC = 46
	keyV = 47
	keyB = 48
	keyN = 49
	keyM = 50

	keyComma      = 51
	keyDot        = 52
	keySlash      = 53
	keyRightshift = 54
	keyLeftalt    = 56
	keySpace      = 57
	keyCapslock   = 58

	keyF1  = 59
	keyF2  = 60
	keyF3  = 61
	keyF4  = 62
	keyF5  = 63
	keyF6  = 64
	keyF7  = 65
	keyF8  = 66
	keyF9  = 67
	keyF10 = 68
	keyF11 = 87
	keyF12 = 88

	keyRightctrl = 97
	keyRightalt  = 100

	keyHome     = 102
	keyUp       = 103
	keyPageup   = 104
	keyLeft     = 105
	keyRight    = 106
	keyEnd      = 107
	keyDown     = 108
	keyPagedown = 109
	keyInsert   = 110
	keyDelete   = 111

	keyLeftmeta  = 125
	keyRightmeta = 126
)

// NameToKeycode maps a platform-independent key name to its Linux keycode.
var NameToKeycode = map[Name]int{
	A: keyA, B: keyB, C: keyC, D: keyD, E: keyE, F: keyF,
	G: keyG, H: keyH, I: keyI, J: keyJ, K: keyK, L: keyL,
	M: keyM, N: keyN, O: keyO, P: keyP, Q: keyQ, R: keyR,
	S: keyS, T: keyT, U: keyU, V: keyV, W: keyW, X: keyX,
	Y: keyY, Z: keyZ,

	N0: key0, N1: key1, N2: key2, N3: key3, N4: key4,
	N5: key5, N6: key6, N7: key7, N8: key8, N9: key9,

	F1: keyF1, F2: keyF2, F3: keyF3, F4: keyF4, F5: keyF5, F6: keyF6,
	F7: keyF7, F8: keyF8, F9: keyF9, F10: keyF10, F11: keyF11, F12: keyF12,

	Enter:      keyEnter,
	Escape:     keyEsc,
	Space:      keySpace,
	Tab:        keyTab,
	Backspace:  keyBackspace,
	Delete:     keyDelete,
	Insert:     keyInsert,
	Home:       keyHome,
	End:        keyEnd,
	PageUp:     keyPageup,
	PageDown:   keyPagedown,
	CapsLock:   keyCapslock,
	ArrowLeft:  keyLeft,
	ArrowRight: keyRight,
	ArrowUp:    keyUp,
	ArrowDown:  keyDown,

	ShiftLeft:    keyLeftshift,
	ShiftRight:   keyRightshift,
	ControlLeft:  keyLeftctrl,
	ControlRight: keyRightctrl,
	AltLeft:      keyLeftalt,
	AltRight:     keyRightalt,
	MetaLeft:     keyLeftmeta,
	MetaRight:    keyRightmeta,

	Minus:        keyMinus,
	Equal:        keyEqual,
	LeftBracket:  keyLeftbrace,
	RightBracket: keyRightbrace,
	Backslash:    keyBackslash,
	Semicolon:    keySemicolon,
	Quote:        keyApostrophe,
	Comma:        keyComma,
	Period:       keyDot,
	Slash:        keySlash,
	Grave:        keyGrave,
}

// KeycodeToName maps a Linux keycode back to its platform-independent name.
var KeycodeToName map[int]Name

func init() {
	KeycodeToName = make(map[int]Name, len(NameToKeycode))
	for name, code := range NameToKeycode {
		KeycodeToName[code] = name
	}
}
