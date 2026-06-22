package config

import (
	"strings"
	"testing"
)

// validClientConfig returns a minimal client *Config that passes Validate():
// role client, a usable network block, a global kcp transport with a key, one
// forward rule (to satisfy the forward/socks5/range requirement), and a single
// valid upstream server. Tests mutate the returned config as needed.
func validClientConfig() *Config {
	return &Config{
		Role: "client",
		Network: NetworkConfig{
			Interface: "eth0",
			IPv4: IPv4Config{
				Addr:      "10.0.0.1:0",
				RouterMAC: "aa:bb:cc:dd:ee:ff",
			},
		},
		Transport: TransportConfig{
			Protocol: "kcp",
			KCP:      KCPConfig{Key: "GLOBAL"},
		},
		Forward: []ForwardRule{
			{Listen: "127.0.0.1:1080", Target: "127.0.0.1:80", Protocol: "tcp"},
		},
		Upstream: &UpstreamConfig{
			Servers: []UpstreamServer{
				{Name: "a", Addr: "1.1.1.1:9000", Key: "KA"},
			},
		},
	}
}

// Validates: Requirements 1.1, 2.1, 3.3
func TestValidateMixedTransportPasses(t *testing.T) {
	cfg := validClientConfig()
	cfg.Upstream.Servers = append(cfg.Upstream.Servers, UpstreamServer{
		Name:      "b",
		Addr:      "2.2.2.2:9000",
		Key:       "KB",
		Transport: &TransportConfig{Protocol: "quic"},
	})
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected mixed kcp/quic config to validate, got: %v", err)
	}
}

// Validates: Requirements 3.1
func TestValidatePerUpstreamProtocolFails(t *testing.T) {
	cfg := validClientConfig()
	cfg.Upstream.Servers = append(cfg.Upstream.Servers, UpstreamServer{
		Name:      "b",
		Addr:      "2.2.2.2:9000",
		Key:       "KB",
		Transport: &TransportConfig{Protocol: "sctp"},
	})
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected invalid per-upstream protocol to fail validation")
	}
	if !strings.Contains(err.Error(), `upstream "b"`) || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("error must name upstream and protocol, got: %v", err)
	}
}

// Validates: Requirements 3.2
func TestValidatePerUpstreamKCPParamsFails(t *testing.T) {
	cfg := validClientConfig()
	cfg.Upstream.Servers = append(cfg.Upstream.Servers, UpstreamServer{
		Name: "b",
		Addr: "2.2.2.2:9000",
		Key:  "KB",
		// Protocol left empty so it inherits the global kcp protocol.
		Transport: &TransportConfig{KCP: KCPConfig{ParityShard: 3, DataShard: 0}},
	})
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected parity-without-data to fail validation")
	}
	if !strings.Contains(err.Error(), `upstream "b"`) || !strings.Contains(err.Error(), "data_shard") {
		t.Fatalf("error must name upstream and data_shard, got: %v", err)
	}
}

// Validates: Requirements 2.1, 3.3
func TestValidateNoOverridesAppliesGlobalDefaults(t *testing.T) {
	cfg := validClientConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected config with no overrides to validate, got: %v", err)
	}
	if cfg.Transport.Conn != 6 {
		t.Errorf("global transport conn default: want 6, got %d", cfg.Transport.Conn)
	}
	if cfg.Transport.KCP.Mode != "fast" {
		t.Errorf("global kcp mode default: want fast, got %q", cfg.Transport.KCP.Mode)
	}
	if cfg.Transport.KCP.MTU != 1150 {
		t.Errorf("global kcp mtu default: want 1150, got %d", cfg.Transport.KCP.MTU)
	}
}
