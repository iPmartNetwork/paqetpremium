package config

import "testing"

func TestValidateClientConfig(t *testing.T) {
	cfg := &Config{
		Role: "client",
		Name: "test",
		Network: NetworkConfig{
			Interface: "eth0",
			IPv4: IPv4Config{
				Addr:      "10.0.0.1:0",
				RouterMAC: "aa:bb:cc:dd:ee:ff",
			},
		},
		Upstream: &UpstreamConfig{
			Servers: []UpstreamServer{{
				Name: "s1",
				Addr: "1.2.3.4:8888",
				Key:  "secret",
			}},
		},
		Forward: []ForwardRule{{
			Listen: "0.0.0.0:443",
			Target: "127.0.0.1:443",
		}},
		Transport: TransportConfig{
			KCP: KCPConfig{Key: "secret"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateIPv6RequiresRouterMAC(t *testing.T) {
	cfg := &Config{
		Role: "server",
		Listen: &ListenConfig{Addr: ":8888"},
		Network: NetworkConfig{
			Interface: "eth0",
			IPv4: IPv4Config{
				Addr:      "10.0.0.1:8888",
				RouterMAC: "aa:bb:cc:dd:ee:ff",
			},
			IPv6: &IPv6Config{Addr: "[2001:db8::1]:8888"},
		},
		Transport: TransportConfig{
			KCP: KCPConfig{Key: "secret"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing ipv6 router_mac")
	}
}

func TestValidateQUICConfig(t *testing.T) {
	cfg := &Config{
		Role: "server",
		Listen: &ListenConfig{Addr: ":8888"},
		Network: NetworkConfig{
			Interface: "eth0",
			IPv4: IPv4Config{
				Addr:      "10.0.0.1:8888",
				RouterMAC: "aa:bb:cc:dd:ee:ff",
			},
		},
		Transport: TransportConfig{
			Protocol: "quic",
			KCP:      KCPConfig{Key: "secret"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate quic: %v", err)
	}
	if cfg.Transport.QUIC.ALPN != "paqetpremium" {
		t.Fatalf("alpn default: %q", cfg.Transport.QUIC.ALPN)
	}
}

func TestUpstreamStrategyNormalization(t *testing.T) {
	u := &UpstreamConfig{Strategy: "round-robin"}
	if u.NormalizedStrategy() != StrategyRoundRobin {
		t.Fatalf("got %q", u.NormalizedStrategy())
	}
}
