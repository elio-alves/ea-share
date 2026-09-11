//go:build windows

package keys

import "testing"

// TestNameToVKRoundTrip checks that every (name, vk) pair NameToVK
// defines maps back to that same name through VKToName. VKToName is not
// a strict inverse (it has a few extra "generic" VK aliases, see the
// comment in keys_windows.go), so this checks the invariant that
// actually matters instead of asserting a strict bijection.
func TestNameToVKRoundTrip(t *testing.T) {
	for name, vk := range NameToVK {
		got, ok := VKToName[vk]
		if !ok {
			t.Errorf("VKToName has no entry for vk 0x%02X (name %s)", vk, name)
			continue
		}
		if got != name {
			t.Errorf("VKToName[0x%02X] = %s, want %s", vk, got, name)
		}
	}
}

// TestNameToVKNoDuplicateCodes catches an accidentally-reused VK constant
// - two different names sharing one code would make the reverse mapping
// silently pick just one of them.
func TestNameToVKNoDuplicateCodes(t *testing.T) {
	seen := make(map[uint32]Name, len(NameToVK))
	for name, vk := range NameToVK {
		if other, ok := seen[vk]; ok {
			t.Errorf("VK code 0x%02X is used by both %s and %s", vk, other, name)
			continue
		}
		seen[vk] = name
	}
}

func TestVKToNameGenericModifierAliases(t *testing.T) {
	// Low-level keyboard hooks normally report the specific left/right VK,
	// but the generic (non-side-specific) codes are mapped too as a
	// fallback - see the comment in keys_windows.go's init().
	cases := map[uint32]Name{
		0x10: ShiftLeft,
		0x11: ControlLeft,
		0x12: AltLeft,
	}
	for vk, want := range cases {
		if got, ok := VKToName[vk]; !ok || got != want {
			t.Errorf("VKToName[0x%02X] = (%s, %v), want (%s, true)", vk, got, ok, want)
		}
	}
}

// TestVKToNameNumpadDigitsAliasTopRow checks that the numeric keypad's
// digit/decimal VKs (VK_NUMPAD0-9, VK_DECIMAL) resolve to the same names
// as the top-row digits/period instead of a separate "NumPad" identity -
// see the comment in keys_windows.go's init() for why (a target's
// character translation of these VKs depends on ITS OWN NumLock state,
// not the source's, so forwarding them as ordinary digits sidesteps that
// entirely instead of forcing the target's NumLock to match).
func TestVKToNameNumpadDigitsAliasTopRow(t *testing.T) {
	cases := map[uint32]Name{
		0x60: N0, 0x61: N1, 0x62: N2, 0x63: N3, 0x64: N4,
		0x65: N5, 0x66: N6, 0x67: N7, 0x68: N8, 0x69: N9,
		0x6E: Period,
	}
	for vk, want := range cases {
		if got, ok := VKToName[vk]; !ok || got != want {
			t.Errorf("VKToName[0x%02X] = (%s, %v), want (%s, true)", vk, got, ok, want)
		}
	}
}
