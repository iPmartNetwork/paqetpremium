package protocol

import (
	"bytes"
	"testing"

	"github.com/paqetpremium/paqetpremium/internal/tunneladdr"
)

func TestEncodeDecodeDatagramRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		flowID  uint64
		payload []byte
	}{
		{"empty", 0, []byte{}},
		{"small", 1, []byte("hello")},
		{"max", 0xDEADBEEFCAFEF00D, bytes.Repeat([]byte{0xAB}, 1172)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			frame := EncodeDatagram(c.flowID, c.payload)
			if len(frame) != DatagramHeaderLen+len(c.payload) {
				t.Fatalf("frame len = %d, want %d", len(frame), DatagramHeaderLen+len(c.payload))
			}
			flowID, payload, ok := DecodeDatagram(frame)
			if !ok {
				t.Fatal("DecodeDatagram returned ok=false")
			}
			if flowID != c.flowID {
				t.Fatalf("flowID = %d, want %d", flowID, c.flowID)
			}
			if !bytes.Equal(payload, c.payload) {
				t.Fatalf("payload mismatch: got %d bytes, want %d", len(payload), len(c.payload))
			}
		})
	}
}

func TestDecodeDatagramShortBuffer(t *testing.T) {
	for n := 0; n < DatagramHeaderLen; n++ {
		if _, _, ok := DecodeDatagram(make([]byte, n)); ok {
			t.Fatalf("DecodeDatagram(len=%d) ok=true, want false", n)
		}
	}
}

func TestUDPDgramMessageRoundTrip(t *testing.T) {
	addr, err := tunneladdr.Parse("example.com:51820")
	if err != nil {
		t.Fatalf("parse addr: %v", err)
	}
	var buf bytes.Buffer
	if err := (&Message{Type: UDPDGRAM, Addr: addr}).Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got Message
	if err := got.Read(&buf); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Type != UDPDGRAM {
		t.Fatalf("Type = %d, want UDPDGRAM (%d)", got.Type, UDPDGRAM)
	}
	if got.Addr == nil || got.Addr.Host != addr.Host || got.Addr.Port != addr.Port {
		t.Fatalf("Addr = %+v, want %+v", got.Addr, addr)
	}
}
