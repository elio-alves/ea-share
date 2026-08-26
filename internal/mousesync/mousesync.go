// Package mousesync implements the wire format for the dedicated,
// UDP-based mouse-position channel used by the controller's optional
// performance mode (-udp-mouse, -edge only; see internal/protocol's
// MsgUDPKey). Unlike internal/protocol's TCP/TLS connection, this channel
// trades reliability/ordering for latency: it carries the controller's
// absolute simulated cursor position plus a sequence number, so a lost or
// reordered packet just means the target skips an intermediate frame
// instead of drifting out of sync - the same principle online games use
// to sync entity position over UDP.
//
// There is no TLS on this channel. Trust is derived from the main
// connection instead: the target generates a random per-session key and
// sends it to the controller over the already-authenticated TCP
// connection (MsgUDPKey); every UDP packet is then authenticated with an
// HMAC over that key, so a packet can't be forged or replayed from a
// different/past session without it. The payload itself (a screen
// coordinate) isn't encrypted - it isn't sensitive enough to justify the
// extra cost on a channel meant to be fast.
package mousesync

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

// KeySize is the length, in bytes, of the per-session authentication key
// exchanged over MsgUDPKey.
const KeySize = 32

// PacketSize is the fixed size of every packet on this channel: an 8-byte
// sequence number, two 4-byte coordinates, and a 32-byte HMAC-SHA256 tag.
const PacketSize = 8 + 4 + 4 + sha256.Size

// MouseAddr derives the UDP mouse channel's address from the main
// mouse/keyboard connection's address: same host, port+2 (the clipboard
// channel, internal/clipsync, already takes port+1). Keeping it on its
// own port means it needs no framing to distinguish it from either of the
// other two connections.
func MouseAddr(addr string) (string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("mousesync: parsing address %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("mousesync: parsing port %q: %w", portStr, err)
	}
	return net.JoinHostPort(host, strconv.Itoa(port+2)), nil
}

// EncodePacket builds one authenticated packet reporting the absolute
// cursor position (x, y) at sequence number seq.
func EncodePacket(key []byte, seq uint64, x, y int32) []byte {
	buf := make([]byte, PacketSize)
	binary.BigEndian.PutUint64(buf[0:8], seq)
	binary.BigEndian.PutUint32(buf[8:12], uint32(x))
	binary.BigEndian.PutUint32(buf[12:16], uint32(y))

	mac := hmac.New(sha256.New, key)
	mac.Write(buf[:16])
	copy(buf[16:], mac.Sum(nil))
	return buf
}

// DecodePacket verifies and parses a packet built by EncodePacket. ok is
// false if buf is the wrong size or the HMAC tag doesn't match key - the
// caller should silently drop the packet in either case.
func DecodePacket(key []byte, buf []byte) (seq uint64, x, y int32, ok bool) {
	if len(buf) != PacketSize {
		return 0, 0, 0, false
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(buf[:16])
	if !hmac.Equal(mac.Sum(nil), buf[16:]) {
		return 0, 0, 0, false
	}

	seq = binary.BigEndian.Uint64(buf[0:8])
	x = int32(binary.BigEndian.Uint32(buf[8:12]))
	y = int32(binary.BigEndian.Uint32(buf[12:16]))
	return seq, x, y, true
}
