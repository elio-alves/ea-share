package clipsync

import (
	"bytes"
	"testing"
)

func TestWriteReadFrameRoundTrip(t *testing.T) {
	cases := []struct {
		kind    Kind
		payload []byte
	}{
		{KindAuth, []byte("hunter2")},
		{KindAuthOK, nil},
		{KindAuthFail, nil},
		{KindRequest, nil},
		{KindText, []byte("hello, clipboard")},
		{KindImage, bytes.Repeat([]byte{0xAB}, 4096)},
		{KindText, []byte{}}, // empty text: "nothing on the clipboard"
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := WriteFrame(&buf, c.kind, c.payload); err != nil {
			t.Fatalf("WriteFrame(kind=%d, %d bytes): %v", c.kind, len(c.payload), err)
		}
		gotKind, gotPayload, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame after writing kind=%d: %v", c.kind, err)
		}
		if gotKind != c.kind {
			t.Errorf("kind = %d, want %d", gotKind, c.kind)
		}
		if !bytes.Equal(gotPayload, c.payload) && !(len(gotPayload) == 0 && len(c.payload) == 0) {
			t.Errorf("payload = %v, want %v", gotPayload, c.payload)
		}
	}
}

func TestWriteFrameTooLarge(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, KindImage, make([]byte, MaxFrameSize+1)); err == nil {
		t.Fatal("expected an error writing an oversized frame, got nil")
	}
}

func TestReadFrameRejectsOversizedLengthPrefix(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte(byte(KindImage))
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // claims a ~4GB payload
	if _, _, err := ReadFrame(&buf); err == nil {
		t.Fatal("expected an error reading an oversized length prefix, got nil")
	}
}

func TestReadFrameTruncated(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, KindText, []byte("hello")); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	truncated := buf.Bytes()[:buf.Len()-2]
	if _, _, err := ReadFrame(bytes.NewReader(truncated)); err == nil {
		t.Fatal("expected an error reading a truncated frame, got nil")
	}
}

func TestClipAddr(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{":7777", ":7778"},
		{"192.168.1.16:7777", "192.168.1.16:7778"},
		{"target.example.com:9000", "target.example.com:9001"},
	}
	for _, c := range cases {
		got, err := ClipAddr(c.in)
		if err != nil {
			t.Fatalf("ClipAddr(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ClipAddr(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClipAddrInvalid(t *testing.T) {
	cases := []string{"", "no-port", "host:not-a-number"}
	for _, in := range cases {
		if _, err := ClipAddr(in); err == nil {
			t.Errorf("ClipAddr(%q): expected an error, got nil", in)
		}
	}
}
