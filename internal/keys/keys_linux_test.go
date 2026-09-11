//go:build linux

package keys

import "testing"

func TestNameToKeycodeRoundTrip(t *testing.T) {
	for name, code := range NameToKeycode {
		got, ok := KeycodeToName[code]
		if !ok {
			t.Errorf("KeycodeToName has no entry for code %d (name %s)", code, name)
			continue
		}
		if got != name {
			t.Errorf("KeycodeToName[%d] = %s, want %s", code, got, name)
		}
	}
}

func TestNameToKeycodeNoDuplicates(t *testing.T) {
	seen := make(map[int]Name, len(NameToKeycode))
	for name, code := range NameToKeycode {
		if other, ok := seen[code]; ok {
			t.Errorf("keycode %d is used by both %s and %s", code, other, name)
			continue
		}
		seen[code] = name
	}
}

// TestKeycodeToNameNumpadDigitsAliasTopRow checks that the numeric
// keypad's digit/decimal keycodes resolve to the same names as the
// top-row digits/period instead of a separate "NumPad" identity - see
// the comment in keys_linux.go's init() for why. This is also why
// KeycodeToName is intentionally larger than NameToKeycode (no longer a
// strict bijection): these are extra reverse-only aliases, same idea as
// keys_windows.go's generic modifier aliases.
func TestKeycodeToNameNumpadDigitsAliasTopRow(t *testing.T) {
	cases := map[int]Name{
		keyKp0: N0, keyKp1: N1, keyKp2: N2, keyKp3: N3, keyKp4: N4,
		keyKp5: N5, keyKp6: N6, keyKp7: N7, keyKp8: N8, keyKp9: N9,
		keyKpdot: Period,
	}
	for code, want := range cases {
		if got, ok := KeycodeToName[code]; !ok || got != want {
			t.Errorf("KeycodeToName[%d] = (%s, %v), want (%s, true)", code, got, ok, want)
		}
	}
}
