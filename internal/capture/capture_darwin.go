//go:build darwin

package capture

import "errors"

// darwinSource is a placeholder: macOS input capture requires a CGEventTap
// via CGO (Quartz Event Services), which isn't implemented yet.
type darwinSource struct{}

// New returns a capture.Source stub for macOS. Start always fails; macOS
// support is not implemented yet (would require a CGEventTap via CGO).
func New() Source {
	return darwinSource{}
}

func (darwinSource) Start() (<-chan Event, error) {
	return nil, errors.New("capture: macOS is not supported yet (requires a CGO-based CGEventTap implementation)")
}

func (darwinSource) Stop() error { return nil }
