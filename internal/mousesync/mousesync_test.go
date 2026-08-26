package mousesync

import (
	"bytes"
	"testing"
)

func testKey() []byte {
	return bytes.Repeat([]byte{0x42}, KeySize)
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cases := []struct {
		seq  uint64
		x, y int32
	}{
		{0, 0, 0},
		{1, 100, 200},
		{1<<64 - 1, -1, -1},
		{42, 1920, 1080},
	}
	key := testKey()
	for _, c := range cases {
		buf := EncodePacket(key, c.seq, c.x, c.y)
		if len(buf) != PacketSize {
			t.Fatalf("EncodePacket(seq=%d): len = %d, want %d", c.seq, len(buf), PacketSize)
		}
		gotSeq, gotX, gotY, ok := DecodePacket(key, buf)
		if !ok {
			t.Fatalf("DecodePacket(seq=%d): ok = false, want true", c.seq)
		}
		if gotSeq != c.seq || gotX != c.x || gotY != c.y {
			t.Errorf("DecodePacket(seq=%d) = (%d, %d, %d), want (%d, %d, %d)", c.seq, gotSeq, gotX, gotY, c.seq, c.x, c.y)
		}
	}
}

func TestDecodePacketWrongKey(t *testing.T) {
	buf := EncodePacket(testKey(), 1, 10, 20)
	wrongKey := bytes.Repeat([]byte{0x99}, KeySize)
	if _, _, _, ok := DecodePacket(wrongKey, buf); ok {
		t.Fatal("DecodePacket with the wrong key: ok = true, want false")
	}
}

func TestDecodePacketTampered(t *testing.T) {
	key := testKey()
	buf := EncodePacket(key, 1, 10, 20)
	buf[8] ^= 0xFF // flip a bit in the x coordinate
	if _, _, _, ok := DecodePacket(key, buf); ok {
		t.Fatal("DecodePacket with a tampered payload: ok = true, want false")
	}
}

func TestDecodePacketWrongSize(t *testing.T) {
	key := testKey()
	buf := EncodePacket(key, 1, 10, 20)
	cases := [][]byte{
		buf[:len(buf)-1],
		append(buf, 0x00),
		nil,
		{},
	}
	for _, c := range cases {
		if _, _, _, ok := DecodePacket(key, c); ok {
			t.Errorf("DecodePacket(%d bytes): ok = true, want false", len(c))
		}
	}
}

func TestMouseAddr(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{":7777", ":7779"},
		{"192.168.1.16:7777", "192.168.1.16:7779"},
		{"target.example.com:9000", "target.example.com:9002"},
	}
	for _, c := range cases {
		got, err := MouseAddr(c.in)
		if err != nil {
			t.Fatalf("MouseAddr(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("MouseAddr(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMouseAddrInvalid(t *testing.T) {
	cases := []string{"", "no-port", "host:not-a-number"}
	for _, in := range cases {
		if _, err := MouseAddr(in); err == nil {
			t.Errorf("MouseAddr(%q): expected an error, got nil", in)
		}
	}
}
