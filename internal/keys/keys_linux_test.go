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

func TestNameToKeycodeBijective(t *testing.T) {
	if len(NameToKeycode) != len(KeycodeToName) {
		t.Errorf("NameToKeycode has %d entries, KeycodeToName has %d - expected a strict bijection",
			len(NameToKeycode), len(KeycodeToName))
	}
}
