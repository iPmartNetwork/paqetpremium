package protocol

import (
	"bytes"
	"io"
	"testing"
)

func TestDatagramRoundTrip(t *testing.T) {
	cases := [][]byte{
		{},
		[]byte("hello"),
		bytes.Repeat([]byte{0xAB}, 1),
		bytes.Repeat([]byte{0x5C}, 1400),
		bytes.Repeat([]byte{0x7E}, MaxDatagram),
	}
	var buf bytes.Buffer
	for _, in := range cases {
		if err := WriteDatagram(&buf, in); err != nil {
			t.Fatalf("WriteDatagram(len=%d): %v", len(in), err)
		}
	}
	out := make([]byte, MaxDatagram)
	for i, want := range cases {
		n, err := ReadDatagram(&buf, out)
		if err != nil {
			t.Fatalf("ReadDatagram #%d: %v", i, err)
		}
		if n != len(want) || !bytes.Equal(out[:n], want) {
			t.Fatalf("datagram #%d mismatch: got %d bytes, want %d", i, n, len(want))
		}
	}
}

// Boundaries are preserved even when multiple datagrams are concatenated in the
// stream (the core bug this framing fixes).
func TestDatagramBoundariesPreserved(t *testing.T) {
	var buf bytes.Buffer
	a := []byte("AAAA")
	b := []byte("BBBBBBBB")
	_ = WriteDatagram(&buf, a)
	_ = WriteDatagram(&buf, b)

	out := make([]byte, 64)
	n1, _ := ReadDatagram(&buf, out)
	if string(out[:n1]) != "AAAA" {
		t.Fatalf("first datagram = %q, want AAAA", out[:n1])
	}
	n2, _ := ReadDatagram(&buf, out)
	if string(out[:n2]) != "BBBBBBBB" {
		t.Fatalf("second datagram = %q, want BBBBBBBB", out[:n2])
	}
}

func TestReadDatagramTooLargeForBuffer(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteDatagram(&buf, bytes.Repeat([]byte{1}, 100))
	small := make([]byte, 10)
	if _, err := ReadDatagram(&buf, small); err == nil {
		t.Fatal("expected error when datagram exceeds buffer")
	}
}

func TestReadDatagramEOF(t *testing.T) {
	if _, err := ReadDatagram(bytes.NewReader(nil), make([]byte, 16)); err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}
