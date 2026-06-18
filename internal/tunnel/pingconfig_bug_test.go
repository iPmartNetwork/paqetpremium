package tunnel

import (
	"context"
	"runtime"
	"testing"

	"github.com/paqetpremium/paqetpremium/internal/config"
)

// Bug condition EXPLORATION test for Bug 4 — Property 4: PingConfig Reports
// Unreachable Server.
//
// This test encodes the EXPECTED (correct) behavior: when the configured server
// is unreachable, PingConfig must return a non-nil error so the
// app/test.go "kcp ping: server reachable" check reports failure instead of a
// false positive.
//
// ENVIRONMENT LIMITATION (matches the Bug 1 precedent in this spec):
// PingConfig(ctx, cfg) first calls cfg.ResolveNetwork() (which calls
// net.InterfaceByName) and then upstream.NewManager(...), which dials via the
// pcap engine. On this development host (Windows, CGO disabled, no real tunnel
// interface) those steps fail BEFORE the buggy `return nil` is ever reached:
// ResolveNetwork errors on the missing interface, and pcap.Open is Linux-only.
// A runtime assertion of "PingConfig returns a non-nil error" would therefore
// PASS on the unfixed code for the WRONG reason (a pcap/interface error rather
// than a ping failure) — a misleading false signal, not a genuine demonstration
// of the defect.
//
// The actual defect is confirmed by direct code inspection of
// internal/tunnel/client.go: PingConfig builds the upstream Manager and then
// executes `return nil` unconditionally, never calling any Ping/PingWithTimeout,
// so a failed ping round-trip is never surfaced. app/test.go relies on
// PingConfig for its "kcp ping: server reachable" line, so the check passes even
// when the server is unreachable (Requirement 1.5).
//
// This test is therefore guarded to run only on Linux, where the real pcap
// engine and a tunnel interface exist and the defective code path can be
// reached honestly. On all other platforms it is skipped to avoid a false
// signal. The runtime assertion is deferred to a Linux host; the fix lives in
// Task 8.
//
// **Validates: Requirements 1.5**
func TestPingConfig_UnreachableServer_BugCondition(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux pcap engine and a tunnel interface")
	}

	// Minimal client config pointing at an unreachable server. The address is in
	// the RFC 5737 TEST-NET-1 range, which is not routable, so no ping round-trip
	// can succeed.
	cfg := &config.Config{
		Role: config.RoleClient,
		Name: "bug4-exploration",
		Network: config.NetworkConfig{
			Interface: "eth0",
			IPv4: config.IPv4Config{
				Addr:      "10.0.0.2",
				RouterMAC: "00:00:00:00:00:01",
			},
		},
		Server: &config.ServerEndpoint{
			// Unreachable / unroutable destination (RFC 5737 TEST-NET-1).
			Addr: "192.0.2.1:51820",
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

	// EXPECTED behavior: an unreachable server yields a non-nil error.
	// On the unfixed code PingConfig returns nil (false positive) once the ping
	// round-trip path is reachable on Linux — this assertion confirms Bug 4.
	if err := PingConfig(context.Background(), cfg); err == nil {
		t.Fatalf("Bug 4 reproduced: PingConfig returned nil for an unreachable server (192.0.2.1:51820); expected a non-nil error so the \"kcp ping\" check reports failure")
	}
}
