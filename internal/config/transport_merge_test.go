package config

import "testing"

func baseTransport() TransportConfig {
	return TransportConfig{
		Protocol: "kcp",
		Conn:     6,
		KCP: KCPConfig{
			Mode:        "fast",
			Block:       "aes-128-gcm",
			Key:         "GLOBAL",
			MTU:         1150,
			DataShard:   10,
			ParityShard: 3,
			SndWnd:      256,
			RcvWnd:      256,
		},
		QUIC: QUICConfig{
			ALPN:        "paqetpremium",
			IdleTimeout: "30s",
		},
	}
}

func TestMergeTransportNilOverride(t *testing.T) {
	base := baseTransport()
	got := mergeTransport(base, nil)
	if got != base {
		t.Fatalf("nil override should return base unchanged: got %+v want %+v", got, base)
	}
}

func TestMergeTransportPartialOverlay(t *testing.T) {
	base := baseTransport()
	override := &TransportConfig{Protocol: "quic"}
	got := mergeTransport(base, override)
	if got.Protocol != "quic" {
		t.Fatalf("protocol not overridden: got %q", got.Protocol)
	}
	// Unset fields inherited from base.
	if got.KCP.MTU != base.KCP.MTU {
		t.Errorf("KCP.MTU should inherit base: got %d want %d", got.KCP.MTU, base.KCP.MTU)
	}
	if got.KCP.Mode != base.KCP.Mode {
		t.Errorf("KCP.Mode should inherit base: got %q want %q", got.KCP.Mode, base.KCP.Mode)
	}
	if got.KCP.Key != base.KCP.Key {
		t.Errorf("KCP.Key should inherit base: got %q want %q", got.KCP.Key, base.KCP.Key)
	}
	if got.Conn != base.Conn {
		t.Errorf("Conn should inherit base: got %d want %d", got.Conn, base.Conn)
	}
}

func TestMergeTransportFullerOverride(t *testing.T) {
	base := baseTransport()
	override := &TransportConfig{
		Protocol: "quic",
		KCP:      KCPConfig{MTU: 1300},
		QUIC:     QUICConfig{ALPN: "custom-alpn"},
	}
	got := mergeTransport(base, override)
	if got.Protocol != "quic" {
		t.Errorf("protocol: got %q want quic", got.Protocol)
	}
	if got.KCP.MTU != 1300 {
		t.Errorf("KCP.MTU: got %d want 1300", got.KCP.MTU)
	}
	if got.QUIC.ALPN != "custom-alpn" {
		t.Errorf("QUIC.ALPN: got %q want custom-alpn", got.QUIC.ALPN)
	}
	// Unset fields still inherited.
	if got.KCP.Mode != base.KCP.Mode {
		t.Errorf("KCP.Mode should inherit base: got %q want %q", got.KCP.Mode, base.KCP.Mode)
	}
	if got.KCP.Key != base.KCP.Key {
		t.Errorf("KCP.Key should inherit base: got %q want %q", got.KCP.Key, base.KCP.Key)
	}
	if got.QUIC.IdleTimeout != base.QUIC.IdleTimeout {
		t.Errorf("QUIC.IdleTimeout should inherit base: got %q want %q", got.QUIC.IdleTimeout, base.QUIC.IdleTimeout)
	}
}

func TestMergeTransportZeroValueInheritance(t *testing.T) {
	base := baseTransport()
	override := &TransportConfig{KCP: KCPConfig{MTU: 0}}
	got := mergeTransport(base, override)
	if got.KCP.MTU != base.KCP.MTU {
		t.Fatalf("zero MTU override should keep base: got %d want %d", got.KCP.MTU, base.KCP.MTU)
	}
}

func TestUpstreamEndpointsPerServerTransport(t *testing.T) {
	cfg := &Config{
		Role: RoleClient,
		Transport: TransportConfig{
			Protocol: "kcp",
			KCP:      KCPConfig{Key: "GLOBAL", MTU: 1150},
		},
		Upstream: &UpstreamConfig{
			Servers: []UpstreamServer{
				{Name: "a", Addr: "1.1.1.1:1000", Key: ""},
				{Name: "b", Addr: "2.2.2.2:2000", Key: "BKEY",
					Transport: &TransportConfig{Protocol: "quic"}},
			},
		},
	}
	eps, err := cfg.UpstreamEndpoints()
	if err != nil {
		t.Fatalf("UpstreamEndpoints: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(eps))
	}
	// Priority ordering preserved (by index default: a=1, b=2).
	if eps[0].Name != "a" || eps[1].Name != "b" {
		t.Fatalf("priority ordering not preserved: got %q, %q", eps[0].Name, eps[1].Name)
	}
	if eps[0].Priority != 1 || eps[1].Priority != 2 {
		t.Errorf("default priorities by index wrong: got %d, %d", eps[0].Priority, eps[1].Priority)
	}

	// Server "a": inherits global protocol and global key.
	if eps[0].Transport.Protocol != "kcp" {
		t.Errorf("a protocol: got %q want kcp", eps[0].Transport.Protocol)
	}
	if eps[0].Transport.KCP.Key != "GLOBAL" {
		t.Errorf("a key: got %q want GLOBAL", eps[0].Transport.KCP.Key)
	}

	// Server "b": protocol override quic, per-server key precedence.
	if eps[1].Transport.Protocol != "quic" {
		t.Errorf("b protocol: got %q want quic", eps[1].Transport.Protocol)
	}
	if eps[1].Transport.KCP.Key != "BKEY" {
		t.Errorf("b key: got %q want BKEY", eps[1].Transport.KCP.Key)
	}
	// MTU still inherited from global.
	if eps[1].Transport.KCP.MTU != 1150 {
		t.Errorf("b MTU: got %d want 1150 (inherited)", eps[1].Transport.KCP.MTU)
	}
}

func TestUpstreamEndpointsLegacySingleServer(t *testing.T) {
	cfg := &Config{
		Role: RoleClient,
		Transport: TransportConfig{
			Protocol: "kcp",
			KCP:      KCPConfig{Key: "GLOBAL", MTU: 1150},
		},
		Server: &ServerEndpoint{Addr: "3.3.3.3:3000"},
	}
	eps, err := cfg.UpstreamEndpoints()
	if err != nil {
		t.Fatalf("UpstreamEndpoints: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(eps))
	}
	if eps[0].Transport.Protocol != cfg.Transport.Protocol {
		t.Errorf("legacy protocol: got %q want %q", eps[0].Transport.Protocol, cfg.Transport.Protocol)
	}
	if eps[0].Transport.KCP.Key != cfg.Transport.KCP.Key {
		t.Errorf("legacy key: got %q want %q", eps[0].Transport.KCP.Key, cfg.Transport.KCP.Key)
	}
}
