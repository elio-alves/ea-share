// Package auth provides a constant-time shared-token check used after the
// TLS handshake to authorize a controller before the target accepts any
// input events from it.
package auth

import "crypto/subtle"

// TokensEqual reports whether got matches want using a constant-time
// comparison, avoiding timing side-channels on token verification.
func TokensEqual(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
