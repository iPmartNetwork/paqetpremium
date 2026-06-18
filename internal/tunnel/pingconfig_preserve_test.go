package tunnel

import (
	"context"
	"runtime"
	"testing"

	"github.com/paqetpremium/paqetpremium/internal/config"
)

// PRESERVATION test for the reachable-server ping path — Property 8 (ping
// portion): Reachable Ping And Matching-Secret Handshake.
//
// PRESERVATION CONTRACT (Requirement 3.4):
//   When PingConfig is invoked with a *reachable* server, it must CONTINUE TO
//   return nil so the app/test.go "kcp ping: server reachable" check still
//   passes after the Bug 4 fix (Task 8). The fix changes the unreachable path
//   (return a non-nil error) but must NOT regress the reachable path, which must
//   stay nil.
//
// ENVIRONMENT LIMITATION (matches the Bug 1 / Task 6 precedent in this spec):
//   PingConfig(ctx, cfg) first calls cfg.ResolveNetwork() (which calls
//   net.InterfaceByName and requires a real tunnel interface) and then builds
//   upstream.NewManager(...), which dials via the Linux-only, CGO-backed pcap
//   engine and performs a real ping round-trip against a live server. On this
//   development host (Windows, CGO disabled, no tunnel interface, no live
//   reachable server) the reachable path simply cannot be exercised:
//   ResolveNetwork fails on the missing interface and pcap.Open is Linux-only.
//
//   Standing up a genuinely reachable pcap-based KCP server is impractical even
//   on Linux inside a unit test: it requires root privileges, a configured
//   tunnel network interface, and a live peer answering on the pcap port.
//   Fabricating a passing assertion here (e.g. asserting nil after a
//   ResolveNetwork/pcap error) would be a false signal — it would "pass" for the
//   wrong reason and would not actually exercise the reachable ping path.
//
//   We therefore deliberately do NOT fabricate a passing assertion. The test is
//   guarded to skip on non-Linux hosts, and on Linux it additionally skips with
//   a clear message that it requires a privileged integration environment
//   (root + tunnel interface + a live reachable server). This locks in the
//   EXPECTED preservation contract and leaves a clearly documented placeholder
//   that integration testing on a real VPS can flesh out with an actual
//   reachable endpoint.
//
// SCOPE NOTE: Property 8 also covers the matching-secret QUIC/KCP handshake
//   portion. That portion is verified separately by the transport tests
//   (Task 10), not here. This file covers only the reachable-ping portion of
//   Property 8 (Requirement 3.4).
//
// **Validates: Requirements 3.4**
func TestPingConfig_ReachableServer_Preservation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux pcap engine, a tunnel interface, and a live reachable server")
	}

	// On Linux the reachable path can only be exercised in a privileged
	// integration environment: it needs root, a real tunnel network interface,
	// and a live server answering the ping round-trip. A unit test cannot stand
	// that up, so we skip rather than fabricate a passing assertion that does not
	// actually exercise the reachable path. A real VPS integration run can
	// replace this skip with a live reachable endpoint and assert the contract
	// below.
	t.Skip("requires a privileged integration environment (root + tunnel interface + live reachable server)")

	// EXPECTED preservation contract, deferred to a privileged integration host:
	//   a reachable server => PingConfig returns nil so the
	//   "kcp ping: server reachable" check passes (Requirement 3.4).
	cfg := &config.Config{
		Role: config.RoleClient,
		Name: "preserve-reachable-ping",
		Network: config.NetworkConfig{
			Interface: "eth0",
			IPv4: config.IPv4Config{
				Addr:      "10.0.0.2",
				RouterMAC: "00:00:00:00:00:01",
			},
		},
		Server: &config.ServerEndpoint{
			// Integration: point this at a live, reachable tunnel server.
			Addr: "127.0.0.1:51820",
		},
		Transport: config.TransportConfig{
			Protocol: "kcp",
			Conn:     1,
			KCP: config.KCPConfig{
				Mode:  "fast",
				Block: "aes-128-gcm",
				Key:   "shared-secret-for-test",
				MTU:   1150,
			},
		},
		Forward: []config.ForwardRule{
			{Listen: "127.0.0.1:1080", Target: "127.0.0.1:80", Protocol: "tcp"},
		},
	}

	if err := PingConfig(context.Background(), cfg); err != nil {
		t.Fatalf("preservation violated: PingConfig returned %v for a reachable server; expected nil so the \"kcp ping: server reachable\" check passes", err)
	}
}
