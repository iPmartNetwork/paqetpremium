package tunnel

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/protocol"
	"github.com/paqetpremium/paqetpremium/internal/transport"
	"github.com/paqetpremium/paqetpremium/internal/tunneladdr"
	"github.com/xtaci/smux"
)

func integrationOptions(t *testing.T, proto string) transport.Options {
	return integrationOptionsShards(t, proto, 0, 0)
}

func integrationOptionsShards(t *testing.T, proto string, dataShard, parityShard int) transport.Options {
	t.Helper()
	opt, err := transport.OptionsFromConfig(config.RoleServer, config.TransportConfig{
		Protocol: proto,
		Conn:     1,
		KCP: config.KCPConfig{
			Mode:        "fast",
			Block:       "aes-128-gcm",
			Key:         "integration-secret-key",
			MTU:         1350,
			DataShard:   dataShard,
			ParityShard: parityShard,
		},
		QUIC: config.QUICConfig{ALPN: "paqetpremium-test"},
	})
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	return opt
}

// runServerLoop accepts tunnel sessions/streams and dispatches each to the relay,
// mirroring Server.serveSession/handleStream without the pcap layer.
func runServerLoop(ctx context.Context, ln *transport.Listener) {
	for {
		sess, err := ln.Accept(ctx)
		if err != nil {
			return
		}
		go func(sess *transport.Session) {
			defer sess.Close()
			for {
				strm, err := sess.Smux.AcceptStream()
				if err != nil {
					return
				}
				go func(strm *smux.Stream) {
					defer strm.Close()
					var msg protocol.Message
					if err := msg.Read(strm); err != nil {
						return
					}
					switch msg.Type {
					case protocol.Ping:
						_ = (&protocol.Message{Type: protocol.Pong}).Write(strm)
					case protocol.TCP:
						if msg.Addr != nil {
							_ = relayTCP(ctx, strm, msg.Addr.String())
						}
					case protocol.UDP:
						if msg.Addr != nil {
							_ = relayUDP(ctx, strm, msg.Addr.String())
						}
					}
				}(strm)
			}
		}(sess)
	}
}

func dialTunnel(ctx context.Context, t *testing.T, proto string) *transport.Session {
	return dialTunnelOpt(ctx, t, integrationOptions(t, proto))
}

// dialTunnelOpt stands up a server listener + client session over a pair of
// loopback UDP PacketConns (no pcap engine required) and returns the client session.
func dialTunnelOpt(ctx context.Context, t *testing.T, opt transport.Options) *transport.Session {
	t.Helper()

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
	go runServerLoop(ctx, ln)

	sess, err := transport.Dial(ctx, serverAddr, opt, clientPC)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// assertTCPEcho opens a stream, points it at a fresh TCP echo server through the
// relay, and verifies a payload round-trips intact.
func assertTCPEcho(ctx context.Context, t *testing.T, sess *transport.Session) {
	t.Helper()

	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	defer echoLn.Close()
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { defer c.Close(); _, _ = io.Copy(c, c) }(c)
		}
	}()

	strm, err := sess.Smux.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer strm.Close()

	addr, err := tunneladdr.Parse(echoLn.Addr().String())
	if err != nil {
		t.Fatalf("parse addr: %v", err)
	}
	if err := (&protocol.Message{Type: protocol.TCP, Addr: addr}).Write(strm); err != nil {
		t.Fatalf("write tcp msg: %v", err)
	}

	want := []byte("hello-through-the-tunnel")
	_ = strm.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := strm.Write(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := make([]byte, len(want))
	_ = strm.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.ReadFull(strm, got); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo mismatch: got %q want %q", got, want)
	}
}

func TestIntegrationTCPEcho(t *testing.T) {
	for _, proto := range []string{"kcp", "quic"} {
		proto := proto
		t.Run(proto, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			sess := dialTunnel(ctx, t, proto)
			assertTCPEcho(ctx, t, sess)
		})
	}
}

func TestIntegrationKCPWithFEC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sess := dialTunnelOpt(ctx, t, integrationOptionsShards(t, "kcp", 10, 3))
	assertTCPEcho(ctx, t, sess)
}

func TestIntegrationUDPDatagrams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	udpEcho, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("udp echo: %v", err)
	}
	defer udpEcho.Close()
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

	sess := dialTunnel(ctx, t, "kcp")
	strm, err := sess.Smux.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer strm.Close()

	addr, err := tunneladdr.Parse(udpEcho.LocalAddr().String())
	if err != nil {
		t.Fatalf("parse addr: %v", err)
	}
	if err := (&protocol.Message{Type: protocol.UDP, Addr: addr}).Write(strm); err != nil {
		t.Fatalf("write udp msg: %v", err)
	}

	payloads := [][]byte{
		[]byte("a"),
		bytes.Repeat([]byte{0x42}, 1200),
		[]byte("third-datagram"),
	}
	buf := make([]byte, 64*1024)
	for i, want := range payloads {
		if err := protocol.WriteDatagram(strm, want); err != nil {
			t.Fatalf("write datagram %d: %v", i, err)
		}
		_ = strm.SetReadDeadline(time.Now().Add(10 * time.Second))
		n, err := protocol.ReadDatagram(strm, buf)
		if err != nil {
			t.Fatalf("read datagram %d: %v", i, err)
		}
		if !bytes.Equal(buf[:n], want) {
			t.Fatalf("datagram %d boundary mismatch: got %d bytes want %d", i, n, len(want))
		}
	}
}
