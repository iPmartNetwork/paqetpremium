package tunnelpool

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/protocol"
	"github.com/paqetpremium/paqetpremium/internal/transport"
	"github.com/xtaci/smux"
)

func poolTestOptions(t *testing.T) transport.Options {
	t.Helper()
	opt, err := transport.OptionsFromConfig(config.RoleServer, config.TransportConfig{
		Protocol: "kcp",
		Conn:     1,
		KCP:      config.KCPConfig{Mode: "fast", Block: "aes-128-gcm", Key: "pool-test-key", MTU: 1350},
	})
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	return opt
}

// startPongServer accepts tunnel sessions over loopback UDP and answers each
// stream's Ping with a Pong.
func startPongServer(ctx context.Context, t *testing.T) (*net.UDPAddr, func()) {
	t.Helper()
	serverPC, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("server packetconn: %v", err)
	}
	ln, err := transport.Listen(poolTestOptions(t), serverPC)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			sess, err := ln.Accept(ctx)
			if err != nil {
				return
			}
			go func(sess *transport.Session) {
				for {
					strm, err := sess.Smux.AcceptStream()
					if err != nil {
						return
					}
					go func(strm *smux.Stream) {
						defer strm.Close()
						_ = protocol.ServePing(strm)
					}(strm)
				}
			}(sess)
		}
	}()
	return serverPC.LocalAddr().(*net.UDPAddr), func() { _ = ln.Close() }
}

func dialPoolSession(ctx context.Context, t *testing.T, addr *net.UDPAddr) *transport.Session {
	t.Helper()
	clientPC, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("client packetconn: %v", err)
	}
	sess, err := transport.Dial(ctx, addr, poolTestOptions(t), clientPC)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return sess
}

func TestPoolSkipsDeadSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	addr, stop := startPongServer(ctx, t)
	defer stop()

	s1 := dialPoolSession(ctx, t, addr)
	s2 := dialPoolSession(ctx, t, addr)
	p := &Pool{sessions: []*transport.Session{s1, s2}, udp: make(map[uint64]*smux.Stream)}
	defer p.Close()

	if !p.Alive() {
		t.Fatal("pool should be alive with two sessions")
	}

	// Kill the first session: the pool must still serve via the second.
	_ = s1.Smux.Close()
	if !p.Alive() {
		t.Fatal("pool should still be alive with one live session")
	}
	if err := p.PingWithTimeout(5 * time.Second); err != nil {
		t.Fatalf("ping via live session failed: %v", err)
	}

	// Kill the second too: the pool is now dead.
	_ = s2.Smux.Close()
	if p.Alive() {
		t.Fatal("pool should be dead after all sessions closed")
	}
	if err := p.PingWithTimeout(2 * time.Second); err == nil {
		t.Fatal("ping should fail when all sessions are dead")
	}
}
