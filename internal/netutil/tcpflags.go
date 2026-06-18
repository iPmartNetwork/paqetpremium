package netutil

import (
	"fmt"
	"net"
	"strings"
)

type TCPFlagSet struct {
	FIN, SYN, RST, PSH, ACK, URG, ECE, CWR, NS bool
}

func ParseTCPFlag(s string) (TCPFlagSet, error) {
	var f TCPFlagSet
	for _, ch := range strings.ToUpper(strings.TrimSpace(s)) {
		switch ch {
		case 'F':
			f.FIN = true
		case 'S':
			f.SYN = true
		case 'R':
			f.RST = true
		case 'P':
			f.PSH = true
		case 'A':
			f.ACK = true
		case 'U':
			f.URG = true
		case 'E':
			f.ECE = true
		case 'C':
			f.CWR = true
		case 'N':
			f.NS = true
		default:
			return f, fmt.Errorf("invalid TCP flag %q in %q", ch, s)
		}
	}
	return f, nil
}

func ParseTCPFlags(flags []string, defaultFlag string) ([]TCPFlagSet, error) {
	if len(flags) == 0 {
		flags = []string{defaultFlag}
	}
	out := make([]TCPFlagSet, 0, len(flags))
	for _, f := range flags {
		parsed, err := ParseTCPFlag(f)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}

func ParseUDPAddr(addr string, allowZeroPort bool) (*net.UDPAddr, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", addr, err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port in %q", addr)
	}
	if port == 0 && !allowZeroPort {
		return nil, fmt.Errorf("port 0 not allowed in %q", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP in %q", addr)
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return &net.UDPAddr{IP: ip, Port: port}, nil
}

// AddrKey maps an (IP, port) pair to a uint64 used purely as a Go map key for
// per-peer TCP flag cycles (remoteByPeer). The key is never used to reconstruct
// the address, so a stable, collision-resistant hash is appropriate. We hash the
// canonical 16-byte form (ip.To16()) so IPv4-in-IPv6 and native IPv6 are handled
// uniformly, then fold in the two port bytes — every octet and the port
// contribute independently, eliminating the old IPv4 octet collision and the
// IPv6-collapses-to-zero defect.
func AddrKey(ip net.IP, port int) uint64 {
	// FNV-1a 64-bit.
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	// ip.To16() yields a canonical 16-byte representation, or nil for a
	// malformed IP. When nil, fold only the port so the result is still
	// deterministic.
	if b := ip.To16(); b != nil {
		for _, c := range b {
			h ^= uint64(c)
			h *= prime64
		}
	}
	h ^= uint64(byte(port >> 8))
	h *= prime64
	h ^= uint64(byte(port))
	h *= prime64
	return h
}
