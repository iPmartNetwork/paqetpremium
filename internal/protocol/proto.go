package protocol

import (
	"encoding/gob"
	"io"

	"github.com/paqetpremium/paqetpremium/internal/netutil"
	"github.com/paqetpremium/paqetpremium/internal/tunneladdr"
)

type Type byte

const (
	Ping  Type = 0x01
	Pong  Type = 0x02
	TCPF  Type = 0x03
	TCP   Type = 0x04
	UDP   Type = 0x05
)

type TCPFlags = netutil.TCPFlagSet

type Message struct {
	Type Type
	Addr *tunneladdr.Addr
	TCPF []TCPFlags
}

func (m *Message) Read(r io.Reader) error {
	return gob.NewDecoder(r).Decode(m)
}

func (m *Message) Write(w io.Writer) error {
	return gob.NewEncoder(w).Encode(m)
}

func PingRoundTrip(stream io.ReadWriter) error {
	if err := (&Message{Type: Ping}).Write(stream); err != nil {
		return err
	}
	var resp Message
	if err := resp.Read(stream); err != nil {
		return err
	}
	if resp.Type != Pong {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func ServePing(stream io.ReadWriter) error {
	var msg Message
	if err := msg.Read(stream); err != nil {
		return err
	}
	if msg.Type != Ping {
		return io.ErrUnexpectedEOF
	}
	return (&Message{Type: Pong}).Write(stream)
}
