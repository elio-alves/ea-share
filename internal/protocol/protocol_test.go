package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteReadMessageRoundTrip(t *testing.T) {
	cases := []Message{
		{Type: MsgAuth, Token: "hunter2"},
		{Type: MsgKey, Key: "A", Down: true},
		{Type: MsgMouseMove, DX: -12, DY: 34},
		{Type: MsgMouseButton, Button: "left", Down: false},
		{Type: MsgMouseWheel, Amount: -3},
		{Type: MsgScreenInfo, Width: 1920, Height: 1080},
		{Type: MsgEngage, Edge: EdgeRight, RelPos: 0.75},
		{Type: MsgPing},
	}
	for _, m := range cases {
		var buf bytes.Buffer
		if err := WriteMessage(&buf, m); err != nil {
			t.Fatalf("WriteMessage(%+v): %v", m, err)
		}
		got, err := ReadMessage(&buf)
		if err != nil {
			t.Fatalf("ReadMessage after writing %+v: %v", m, err)
		}
		if got != m {
			t.Errorf("round trip mismatch: wrote %+v, read %+v", m, got)
		}
	}
}

func TestWriteMessageTooLarge(t *testing.T) {
	var buf bytes.Buffer
	huge := Message{Type: MsgKey, Key: strings.Repeat("x", MaxMessageSize+1)}
	if err := WriteMessage(&buf, huge); err == nil {
		t.Fatal("expected an error writing an oversized message, got nil")
	}
}

func TestReadMessageRejectsOversizedLengthPrefix(t *testing.T) {
	var buf bytes.Buffer
	// A length prefix claiming more than MaxMessageSize must be rejected
	// before any attempt to read that many bytes from the peer - this is
	// the guard against a malicious/corrupt peer's claimed length.
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	if _, err := ReadMessage(&buf); err == nil {
		t.Fatal("expected an error reading an oversized length prefix, got nil")
	}
}

func TestReadMessageTruncated(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMessage(&buf, Message{Type: MsgKey, Key: "A", Down: true}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	truncated := buf.Bytes()[:buf.Len()-2]
	if _, err := ReadMessage(bytes.NewReader(truncated)); err == nil {
		t.Fatal("expected an error reading a truncated message, got nil")
	}
}

func TestEdgeOpposite(t *testing.T) {
	cases := map[Edge]Edge{
		EdgeLeft:   EdgeRight,
		EdgeRight:  EdgeLeft,
		EdgeTop:    EdgeBottom,
		EdgeBottom: EdgeTop,
	}
	for edge, want := range cases {
		if got := edge.Opposite(); got != want {
			t.Errorf("%s.Opposite() = %s, want %s", edge, got, want)
		}
		if got := edge.Opposite().Opposite(); got != edge {
			t.Errorf("%s.Opposite().Opposite() = %s, want %s (should be involutive)", edge, got, edge)
		}
	}
}
