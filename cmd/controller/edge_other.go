//go:build !windows

package main

import (
	"errors"
	"net"

	"kbs/internal/protocol"
)

func runEdgeAware(conn net.Conn, edge protocol.Edge, clip *clipClient) error {
	return errors.New("-edge is only supported on Windows right now")
}
