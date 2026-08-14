//go:build !windows

package main

import (
	"crypto/tls"
	"errors"
)

type clipClient struct{}

func (c *clipClient) Close() error { return nil }

func dialClipboard(mainAddr, token string, tlsCfg *tls.Config) (*clipClient, error) {
	return nil, errors.New("clipboard sync (Ctrl+Alt+V) is only supported on Windows right now")
}
