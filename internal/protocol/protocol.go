// Package protocol defines the wire format exchanged between the controller
// (the machine sharing its keyboard/mouse) and the target (the machine that
// receives and injects those events).
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

// MaxMessageSize bounds a single framed message to guard against a
// malicious or corrupt peer claiming an absurd length.
const MaxMessageSize = 64 * 1024

type MsgType string

const (
	MsgAuth        MsgType = "auth"
	MsgAuthOK      MsgType = "auth_ok"
	MsgAuthFail    MsgType = "auth_fail"
	MsgKey         MsgType = "key"
	MsgMouseMove   MsgType = "mouse_move"
	MsgMouseButton MsgType = "mouse_button"
	MsgMouseWheel  MsgType = "mouse_wheel"
	MsgPing        MsgType = "ping"

	// MsgScreenInfo is sent target->controller once, right after auth,
	// reporting the target's screen bounds so the controller can simulate
	// the target's cursor position for edge-switching without needing any
	// further target->controller traffic.
	MsgScreenInfo MsgType = "screen_info"

	// MsgEngage is sent controller->target when the controller's cursor
	// crosses its configured screen edge: the target should warp its
	// cursor to the corresponding entry position and start expecting a
	// stream of relative moves.
	MsgEngage MsgType = "engage"
)

// Edge identifies one side of a screen, used by MsgEngage to say which
// edge the controller's cursor exited (and, on the target, which edge it
// should enter from).
type Edge string

const (
	EdgeLeft   Edge = "left"
	EdgeRight  Edge = "right"
	EdgeTop    Edge = "top"
	EdgeBottom Edge = "bottom"
)

// Opposite returns the facing edge on the other screen, e.g. a cursor
// exiting Right enters the neighboring screen's Left edge.
func (e Edge) Opposite() Edge {
	switch e {
	case EdgeLeft:
		return EdgeRight
	case EdgeRight:
		return EdgeLeft
	case EdgeTop:
		return EdgeBottom
	case EdgeBottom:
		return EdgeTop
	default:
		return e
	}
}

// Message is the single envelope used for every event on the wire.
// Fields are omitted (via omitempty) when not relevant to Type.
type Message struct {
	Type MsgType `json:"type"`

	// MsgAuth
	Token string `json:"token,omitempty"`

	// MsgKey: Key is a name from the keys package (e.g. "A", "Enter").
	Key  string `json:"key,omitempty"`
	Down bool   `json:"down,omitempty"`

	// MsgMouseMove: relative pixel deltas.
	DX int32 `json:"dx,omitempty"`
	DY int32 `json:"dy,omitempty"`

	// MsgMouseButton: "left" | "right" | "middle".
	Button string `json:"button,omitempty"`

	// MsgMouseWheel: positive = scroll up/away from user.
	Amount int32 `json:"amount,omitempty"`

	// MsgScreenInfo: the sender's screen bounds, in pixels.
	Width  int32 `json:"width,omitempty"`
	Height int32 `json:"height,omitempty"`

	// MsgEngage: which edge of the controller's screen the cursor exited,
	// and how far along that edge (0.0-1.0) it crossed.
	Edge   Edge    `json:"edge,omitempty"`
	RelPos float64 `json:"rel_pos,omitempty"`
}

// WriteMessage frames m as a 4-byte big-endian length prefix followed by
// its JSON encoding.
func WriteMessage(w io.Writer, m Message) error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(body) > MaxMessageSize {
		return errors.New("protocol: message too large")
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

// ReadMessage reads one length-prefixed JSON message from r.
func ReadMessage(r io.Reader) (Message, error) {
	var m Message
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return m, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxMessageSize {
		return m, errors.New("protocol: message too large")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return m, err
	}
	err := json.Unmarshal(body, &m)
	return m, err
}
