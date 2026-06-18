package netutil

import (
	"fmt"
	"math/rand"
	"net"
	"testing"
)

// These are PRESERVATION tests for Bugs 2 and 3. Unlike the bug-condition
// exploration tests in tcpflags_bug_test.go, these lock in EXISTING non-buggy
// behavior and therefore MUST PASS on the current unfixed code (and must keep
// passing after the fix).
//
//   - Property 6: AddrKey is deterministic/stable — repeated calls with the same
//     IPv4 (IP, port) pair return an identical key. This holds on both unfixed
//     and fixed code; it is a stability/determinism contract relied on by the
//     remoteByPeer map lookups.
//   - Property 7: ParseUDPAddr parses valid IPv4 correctly and rejects invalid
//     inputs (bad IP, out-of-range port, disallowed port 0, missing port). IPs
//     are compared with net.IP.Equal so the assertions are representation-
//     agnostic and survive the fix.

// Property 6 — Stable IPv4 keys (fixed examples).
//
// Repeated AddrKey calls with the same IPv4 (IP, port) pair must return an
// identical key.
//
// **Validates: Requirements 3.2**
func TestAddrKey_StableIPv4_Preservation(t *testing.T) {
	cases := []struct {
		ip   string
		port int
	}{
		{"10.0.0.1", 8888},
		{"192.168.1.1", 443},
		{"127.0.0.1", 1},
		{"8.8.8.8", 53},
		{"255.255.255.255", 65535},
		{"0.0.0.0", 1234},
	}

	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("test bug: failed to parse %q", tc.ip)
		}
		first := AddrKey(ip, tc.port)
		// Call repeatedly in a small loop; every call must match the first.
		for i := 0; i < 100; i++ {
			got := AddrKey(net.ParseIP(tc.ip), tc.port)
			if got != first {
				t.Fatalf("AddrKey not stable for %s:%d: call %d returned %#x, first returned %#x",
					tc.ip, tc.port, i, got, first)
			}
		}
	}
}

// Property 6 — Stable IPv4 keys (seeded property-style loop).
//
// For random IPv4 (IP, port) pairs, AddrKey(ip, port) == AddrKey(ip, port) on
// repeated calls.
//
// **Validates: Requirements 3.2**
func TestAddrKey_StableIPv4_Property_Preservation(t *testing.T) {
	rng := rand.New(rand.NewSource(0xBADC0DE))

	const total = 2000
	for i := 0; i < total; i++ {
		ipStr := fmt.Sprintf("%d.%d.%d.%d",
			rng.Intn(256), rng.Intn(256), rng.Intn(256), rng.Intn(256))
		port := rng.Intn(65535) + 1

		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("test bug: failed to parse generated IP %q", ipStr)
		}

		k1 := AddrKey(ip, port)
		k2 := AddrKey(ip, port)
		// Also parse a fresh copy to guard against any reliance on slice identity.
		k3 := AddrKey(net.ParseIP(ipStr), port)

		if k1 != k2 || k1 != k3 {
			t.Fatalf("AddrKey not stable for %s:%d: got %#x, %#x, %#x", ipStr, port, k1, k2, k3)
		}
	}
}

// Property 7 — IPv4 parsing and validation unchanged.
//
// Valid IPv4 input parses to the correct IP (compared via net.IP.Equal, which is
// representation-agnostic) and port, and invalid inputs are rejected exactly as
// before.
//
// **Validates: Requirements 3.3**
func TestParseUDPAddr_IPv4AndValidation_Preservation(t *testing.T) {
	// Valid IPv4: correct IP + port, no error.
	addr, err := ParseUDPAddr("10.0.0.1:8888", false)
	if err != nil {
		t.Fatalf("ParseUDPAddr(\"10.0.0.1:8888\", false) returned error: %v", err)
	}
	if want := net.ParseIP("10.0.0.1"); !addr.IP.Equal(want) {
		t.Fatalf("ParseUDPAddr IP = %v, want %v (Equal)", addr.IP, want)
	}
	if addr.Port != 8888 {
		t.Fatalf("ParseUDPAddr port = %d, want 8888", addr.Port)
	}

	// allowZeroPort=true: port 0 accepted.
	addr0, err := ParseUDPAddr("0.0.0.0:0", true)
	if err != nil {
		t.Fatalf("ParseUDPAddr(\"0.0.0.0:0\", true) returned error: %v", err)
	}
	if addr0.Port != 0 {
		t.Fatalf("ParseUDPAddr port = %d, want 0", addr0.Port)
	}
	if want := net.ParseIP("0.0.0.0"); !addr0.IP.Equal(want) {
		t.Fatalf("ParseUDPAddr IP = %v, want %v (Equal)", addr0.IP, want)
	}

	// Rejection cases: each must return a non-nil error.
	rejections := []struct {
		name          string
		addr          string
		allowZeroPort bool
	}{
		{"disallowed port 0", "0.0.0.0:0", false},
		{"invalid IP", "not-an-ip:80", false},
		{"port out of range", "10.0.0.1:70000", false},
		{"missing port", "10.0.0.1", false},
	}
	for _, tc := range rejections {
		got, err := ParseUDPAddr(tc.addr, tc.allowZeroPort)
		if err == nil {
			t.Fatalf("%s: ParseUDPAddr(%q, %v) returned no error (result=%v), want an error",
				tc.name, tc.addr, tc.allowZeroPort, got)
		}
	}
}
