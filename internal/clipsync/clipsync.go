// Package clipsync implements the wire protocol for the dedicated
// clipboard-transfer connection between controller and target (separate
// from internal/protocol's mouse/keyboard connection, so a large clipboard
// payload never delays input events).
//
// Framing is a 1-byte kind tag + 4-byte big-endian length + payload, not
// JSON: a screenshot is easily multiple megabytes, and base64-in-JSON
// would inflate that by another third for no benefit on a connection that
// only ever carries this one kind of payload.
package clipsync

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

// MaxFrameSize bounds a single frame; generous enough for a full-screen
// screenshot PNG while still guarding against a corrupt/hostile peer
// claiming an absurd length.
const MaxFrameSize = 32 * 1024 * 1024

type Kind byte

const (
	KindAuth     Kind = iota // payload: token bytes
	KindAuthOK               // payload: none
	KindAuthFail             // payload: none
	KindRequest              // payload: none — "send me your clipboard"
	KindText                 // payload: UTF-8 text
	KindImage                // payload: PNG bytes
)

// WriteFrame frames kind+payload as a 1-byte kind, 4-byte big-endian
// length, then payload.
func WriteFrame(w io.Writer, kind Kind, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return errors.New("clipsync: frame too large")
	}
	var hdr [5]byte
	hdr[0] = byte(kind)
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

// ClipAddr derives the clipboard channel's address from the main
// mouse/keyboard connection's address: same host, port+1. Keeping the
// clipboard channel on a dedicated port (rather than multiplexed onto the
// main connection) means a large payload on it can never delay an input
// event queued behind it.
func ClipAddr(addr string) (string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("clipsync: parsing address %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("clipsync: parsing port %q: %w", portStr, err)
	}
	return net.JoinHostPort(host, strconv.Itoa(port+1)), nil
}

// ReadFrame reads one frame written by WriteFrame.
func ReadFrame(r io.Reader) (Kind, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > MaxFrameSize {
		return 0, nil, errors.New("clipsync: frame too large")
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return Kind(hdr[0]), payload, nil
}
