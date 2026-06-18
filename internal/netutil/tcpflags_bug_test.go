package netutil

import (
	"fmt"
	"math/rand"
	"net"
	"testing"
)

// These are bug condition EXPLORATION tests for Bugs 2 and 3. They encode the
// EXPECTED (correct) behavior and therefore MUST FAIL on the current unfixed
// code in tcpflags.go. A failure here confirms the bugs exist:
//   - Bug 2: AddrKey mis-packs IPv4 octets (octet[3]<<16 overlaps octet[1]<<16)
//     and returns 0 for every IPv6 address (To4() == nil short-circuit).
//   - Bug 3: ParseUDPAddr stores ip.To4(), which is nil for IPv6 inputs,
//     silently dropping the IPv6 address.

// Bug 2 — Property 2: distinct IPv4 (IP, port) pairs must produce distinct keys.
// Includes the historical collision pair from the design.
//
// **Validates: Requirements 1.2**
func TestAddrKey_DistinctIPv4_BugCondition(t *testing.T) {
	const port = 8888

	a := AddrKey(net.ParseIP("192.0.2.1"), port)
	b := AddrKey(net.ParseIP("192.1.2.0"), port)

	// On unfixed code these collide (octet[3]<<16 overlaps octet[1]<<16),
	// so this assertion fails — confirming Bug 2 for IPv4.
	if a == b {
		t.Fatalf("Bug 2 (IPv4 collision) reproduced: AddrKey(192.0.2.1,%d)=%#x == AddrKey(192.1.2.0,%d)=%#x; expected distinct keys",
			port, a, port, b)
	}
}

// Bug 2 — Property 2: distinct IPv6 inputs must produce distinct keys.
//
// **Validates: Requirements 1.3**
func TestAddrKey_DistinctIPv6_BugCondition(t *testing.T) {
	a := AddrKey(net.ParseIP("2001:db8::1"), 80)
	b := AddrKey(net.ParseIP("2001:db8::2"), 443)

	// On unfixed code both return 0 (To4() == nil short-circuit),
	// so this assertion fails — confirming Bug 2 for IPv6.
	if a == b {
		t.Fatalf("Bug 2 (IPv6 collapse) reproduced: AddrKey(2001:db8::1,80)=%#x == AddrKey(2001:db8::2,443)=%#x; both collapse to the same key (expected distinct)",
			a, b)
	}
}

// Bug 2 — Property 2 (property-based style): distinct (IP, port) pairs must map
// to distinct keys. Generates many random IPv4 and IPv6 pairs with a seeded RNG
// and uses a map[uint64] to detect collisions among distinct inputs.
//
// **Validates: Requirements 1.2, 1.3**
func TestAddrKey_NoCollisions_Property_BugCondition(t *testing.T) {
	rng := rand.New(rand.NewSource(0xC0FFEE))

	type pair struct {
		ip   string
		port int
	}

	const total = 4000
	seen := make(map[uint64]pair, total)
	inputs := make(map[string]struct{}, total)

	collisions := 0
	var firstExample string

	for i := 0; i < total; i++ {
		var ipStr string
		if i%2 == 0 {
			// Random IPv4
			ipStr = fmt.Sprintf("%d.%d.%d.%d",
				rng.Intn(256), rng.Intn(256), rng.Intn(256), rng.Intn(256))
		} else {
			// Random IPv6 (2001:db8::/32 prefix with random low bits)
			ipStr = fmt.Sprintf("2001:db8:%x:%x:%x:%x:%x:%x",
				rng.Intn(0x10000), rng.Intn(0x10000), rng.Intn(0x10000),
				rng.Intn(0x10000), rng.Intn(0x10000), rng.Intn(0x10000))
		}
		port := rng.Intn(65535) + 1

		key := fmt.Sprintf("%s|%d", ipStr, port)
		if _, dup := inputs[key]; dup {
			// Skip genuinely duplicate inputs; we only care about distinct inputs.
			continue
		}
		inputs[key] = struct{}{}

		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("test bug: failed to parse generated IP %q", ipStr)
		}
		k := AddrKey(ip, port)

		if prev, ok := seen[k]; ok {
			collisions++
			if firstExample == "" {
				firstExample = fmt.Sprintf("key=%#x produced by both (%s:%d) and (%s:%d)",
					k, prev.ip, prev.port, ipStr, port)
			}
			continue
		}
		seen[k] = pair{ip: ipStr, port: port}
	}

	// On unfixed code, all IPv6 inputs collapse to 0 (mass collisions) and the
	// IPv4 bit-packing collides too, so collisions > 0 and this assertion fails.
	if collisions > 0 {
		t.Fatalf("Bug 2 reproduced: %d colliding keys among distinct (IP,port) pairs; first example: %s", collisions, firstExample)
	}
}

// Bug 3 — Property 3: ParseUDPAddr must preserve the IPv6 address and port.
//
// **Validates: Requirements 1.4**
func TestParseUDPAddr_IPv6Preserved_BugCondition(t *testing.T) {
	addr, err := ParseUDPAddr("[2001:db8::1]:443", false)
	if err != nil {
		t.Fatalf("unexpected error parsing IPv6 address: %v", err)
	}

	want := net.ParseIP("2001:db8::1")

	// On unfixed code, ip.To4() is nil for IPv6, so addr.IP is nil and these
	// assertions fail — confirming Bug 3.
	if addr.IP == nil {
		t.Fatalf("Bug 3 reproduced: ParseUDPAddr(\"[2001:db8::1]:443\") returned a nil IP (IPv6 silently dropped)")
	}
	if !addr.IP.Equal(want) {
		t.Fatalf("Bug 3 reproduced: ParseUDPAddr IP = %v, want %v", addr.IP, want)
	}
	if addr.Port != 443 {
		t.Fatalf("ParseUDPAddr port = %d, want 443", addr.Port)
	}
}
