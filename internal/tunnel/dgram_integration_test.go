package tunnel

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/protocol"
	"github.com/paqetpremium/paqetpremium/internal/transport"
	"github.com/paqetpremium/paqetpremium/internal/tunneladdr"
)

// TestUDPDatagramRelayLoopback exercises the full client->exit UDP datagram
// round-trip over a loopback QUIC session (no pcap/root required, runs on
// Windows/CI). It mirrors the server's UDPDGRAM handling via dgramHub directly:
// a control stream carries the target Addr and the assigned flowID, and the
// actual UDP payloads ride unreliable QUIC datagrams.
func TestUDPDatagramRelayLoopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	// 1. Local UDP echo server: bounce every datagram back to its sender.
	udpEcho, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("udp echo listen: %v", err)
	}
	t.Cleanup(func() { _ = udpEcho.Close() })
	go func() {
		b := make([]byte, 64*1024)
		for {
			n, a, err := udpEcho.ReadFromUDP(b)
			if err != nil {
				return
			}
			_, _ = udpEcho.WriteToUDP(b[:n], a)
		}
	}()
	echoAddr := udpEcho.LocalAddr().String()

	// 2. Stand up a loopback QUIC transport pair over UDP PacketConns.
	opt := integrationOptions(t, "quic")
	serverPC, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("server packetconn: %v", err)
	}
	clientPC, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("client packetconn: %v", err)
	}
	serverAddr := serverPC.LocalAddr().(*net.UDPAddr)

	ln, err := transport.Listen(opt, serverPC)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	type accepted struct {
		sess *transport.Session
		err  error
	}
	accCh := make(chan accepted, 1)
	go func() {
		s, e := ln.Accept(ctx)
		accCh <- accepted{s, e}
	}()

	cliSess, err := transport.Dial(ctx, serverAddr, opt, clientPC)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = cliSess.Close() })

	a := <-accCh
	if a.err != nil {
		t.Fatalf("accept: %v", a.err)
	}
	srvSess := a.sess
	t.Cleanup(func() { _ = srvSess.Close() })

	if !cliSess.DatagramsOK() || !srvSess.DatagramsOK() {
		t.Fatalf("expected QUIC datagram support on both ends")
	}

	// 3. SERVER side: mirror handleStream's UDPDGRAM case via dgramHub.
	hub := newDgramHub(srvSess, slog.Default())
	go hub.recvLoop(ctx)
	go func() {
		ctrl, err := srvSess.Smux.AcceptStream()
		if err != nil {
			return
		}
		defer ctrl.Close()
		var msg protocol.Message
		if err := msg.Read(ctrl); err != nil {
			return
		}
		if msg.Type != protocol.UDPDGRAM || msg.Addr == nil {
			return
		}
		flowID, conn, err := hub.openFlow(msg.Addr.String())
		if err != nil {
			return
		}
		var hdr [8]byte
		binary.BigEndian.PutUint64(hdr[:], flowID)
		if _, err := ctrl.Write(hdr[:]); err != nil {
			return
		}
		go hub.flowReadTargetToClient(ctx, flowID, conn)
		<-ctx.Done() // keep the control stream (and flow) alive for the test
	}()

	// 4. CLIENT side: open the control stream, announce the target, get flowID.
	ctrl, err := cliSess.Smux.OpenStream()
	if err != nil {
		t.Fatalf("open control stream: %v", err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })

	addr, err := tunneladdr.Parse(echoAddr)
	if err != nil {
		t.Fatalf("parse echo addr: %v", err)
	}
	if err := (&protocol.Message{Type: protocol.UDPDGRAM, Addr: addr}).Write(ctrl); err != nil {
		t.Fatalf("write UDPDGRAM msg: %v", err)
	}
	var fhdr [8]byte
	_ = ctrl.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.ReadFull(ctrl, fhdr[:]); err != nil {
		t.Fatalf("read flowID: %v", err)
	}
	flowID := binary.BigEndian.Uint64(fhdr[:])

	// sendRecv writes one datagram and waits for its boundary-preserved echo.
	// UDP is best-effort even on loopback, so retry a couple of times.
	sendRecv := func(payload []byte) {
		t.Helper()
		for attempt := 0; attempt < 3; attempt++ {
			if err := cliSess.SendDatagram(protocol.EncodeDatagram(flowID, payload)); err != nil {
				t.Fatalf("send datagram (len %d): %v", len(payload), err)
			}
			rctx, rcancel := context.WithTimeout(ctx, 3*time.Second)
			b, err := cliSess.ReceiveDatagram(rctx)
			rcancel()
			if err != nil {
				continue // best-effort: retry the send
			}
			gotID, got, ok := protocol.DecodeDatagram(b)
			if !ok {
				t.Fatalf("decode datagram failed")
			}
			if gotID != flowID {
				t.Fatalf("flowID mismatch: got %d want %d", gotID, flowID)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("payload mismatch (len %d): boundary not preserved", len(payload))
			}
			return
		}
		t.Fatalf("no echo received for payload len %d", len(payload))
	}

	// Boundary-preserving echo across several payload sizes, incl. the max.
	maxPayload := cliSess.MaxDatagramPayload()
	for _, size := range []int{1, 100, 1000, maxPayload} {
		sendRecv(bytes.Repeat([]byte{0x5a}, size))
	}

	// 5. Flow reuse: two datagrams back-to-back on the same flowID, both echoed.
	first := []byte("reuse-one")
	second := []byte("reuse-two-longer-payload")
	if err := cliSess.SendDatagram(protocol.EncodeDatagram(flowID, first)); err != nil {
		t.Fatalf("send reuse first: %v", err)
	}
	if err := cliSess.SendDatagram(protocol.EncodeDatagram(flowID, second)); err != nil {
		t.Fatalf("send reuse second: %v", err)
	}
	seen := map[string]bool{}
	for len(seen) < 2 {
		rctx, rcancel := context.WithTimeout(ctx, 5*time.Second)
		b, err := cliSess.ReceiveDatagram(rctx)
		rcancel()
		if err != nil {
			t.Fatalf("reuse receive: %v (seen %d/2)", err, len(seen))
		}
		gotID, got, ok := protocol.DecodeDatagram(b)
		if !ok || gotID != flowID {
			t.Fatalf("reuse decode/flowID mismatch")
		}
		seen[string(got)] = true
	}
	if !seen[string(first)] || !seen[string(second)] {
		t.Fatalf("flow reuse: missing echoes, seen=%v", seen)
	}

	// 6. Oversized: MaxDatagramPayload() is a conservative floor (payloads up
	// to it always fit), so the true rejection ceiling is the QUIC connection's
	// negotiated max datagram size, which is higher. A payload far beyond any
	// plausible ceiling must be rejected by SendDatagram, documenting that
	// oversized datagrams are dropped at the transport boundary rather than
	// fragmented or silently truncated.
	oversized := make([]byte, 64*1024)
	if err := cliSess.SendDatagram(protocol.EncodeDatagram(flowID, oversized)); err == nil {
		t.Fatalf("expected oversized datagram (payload %d) to be rejected", len(oversized))
	}
}
