package protocol

import "encoding/binary"

// DatagramHeaderLen is the size of the flow header prefixed to a QUIC datagram
// payload: an 8-byte big-endian flow identifier.
const DatagramHeaderLen = 8

// EncodeDatagram frames a payload for transport over a QUIC DATAGRAM frame: an
// 8-byte big-endian flowID followed by the raw payload.
func EncodeDatagram(flowID uint64, payload []byte) []byte {
	out := make([]byte, DatagramHeaderLen+len(payload))
	binary.BigEndian.PutUint64(out[:DatagramHeaderLen], flowID)
	copy(out[DatagramHeaderLen:], payload)
	return out
}

// DecodeDatagram splits a framed datagram back into its flowID and payload.
// ok is false if b is shorter than the 8-byte header. The returned payload
// aliases b and must not be retained beyond the buffer's lifetime.
func DecodeDatagram(b []byte) (flowID uint64, payload []byte, ok bool) {
	if len(b) < DatagramHeaderLen {
		return 0, nil, false
	}
	flowID = binary.BigEndian.Uint64(b[:DatagramHeaderLen])
	return flowID, b[DatagramHeaderLen:], true
}
