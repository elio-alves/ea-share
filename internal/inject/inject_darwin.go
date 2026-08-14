//go:build darwin

package inject

import "errors"

// New is a placeholder for macOS. Injection would require CGEventPost via
// CGO (Quartz Event Services), which isn't implemented yet.
func New() (Injector, error) {
	return nil, errors.New("inject: macOS is not supported yet (requires a CGO-based CGEventPost implementation)")
}
