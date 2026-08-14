package auth

import "testing"

func TestTokensEqual(t *testing.T) {
	cases := []struct {
		got, want string
		equal     bool
	}{
		{"hunter2", "hunter2", true},
		{"hunter2", "hunter3", false},
		{"hunter2", "hunter22", false}, // different length
		{"", "", true},
		{"", "hunter2", false},
		{"Hunter2", "hunter2", false}, // case-sensitive
	}
	for _, c := range cases {
		if got := TokensEqual(c.got, c.want); got != c.equal {
			t.Errorf("TokensEqual(%q, %q) = %v, want %v", c.got, c.want, got, c.equal)
		}
	}
}
