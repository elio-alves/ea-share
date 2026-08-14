//go:build !windows

package main

import (
	"crypto/tls"
	"errors"

	"kbs/internal/inject"
)

func startClipboardListener(cert tls.Certificate, listenAddr string, injector inject.Injector, token string) error {
	return errors.New("clipboard sync (Ctrl+Alt+V) is only supported on Windows right now")
}
