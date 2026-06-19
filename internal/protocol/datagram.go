package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// MaxDatagram is the largest UDP payload we frame (uint16 length prefix).
const MaxDatagram = 65535

// WriteDatagram writes a single length-prefixed datagram: 2-byte big-endian
// length followed by the payload. This preserves UDP datagram boundaries when
// tunneling over a smux byte-stream.
func WriteDatagram(w io.Writer, p []byte) error {
	if len(p) > MaxDatagram {
		return fmt.Errorf("datagram too large: %d", len(p))
	}
	frame := make([]byte, 2+len(p))
	binary.BigEndian.PutUint16(frame[:2], uint16(len(p)))
	copy(frame[2:], p)
	_, err := w.Write(frame)
	return err
}

// ReadDatagram reads one length-prefixed datagram into buf and returns its
// length. It returns an error if the datagram does not fit in buf.
func ReadDatagram(r io.Reader, buf []byte) (int, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:]))
	if n == 0 {
		return 0, nil
	}
	if n > len(buf) {
		return 0, fmt.Errorf("datagram %d exceeds buffer %d", n, len(buf))
	}
	if _, err := io.ReadFull(r, buf[:n]); err != nil {
		return 0, err
	}
	return n, nil
}
