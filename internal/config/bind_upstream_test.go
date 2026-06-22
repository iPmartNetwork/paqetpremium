package config

import (
	"strings"
	"testing"
)

// clientConfigWithForward builds a minimal valid client Config with two named
// upstreams ("DE" and "FN") and the provided forward rules.
func clientConfigWithForward(forward []ForwardRule) *Config {
	return &Config{
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
			Servers: []UpstreamServer{
				{Name: "DE", Addr: "1.2.3.4:9001", Key: "k1"},
				{Name: "FN", Addr: "5.6.7.8:9002", Key: "k2"},
			},
		},
		Forward:   forward,
		Transport: TransportConfig{KCP: KCPConfig{Key: "K"}},
	}
}

func TestBindUpstreamMatches(t *testing.T) {
	cfg := clientConfigWithForward([]ForwardRule{{
		Listen:       "0.0.0.0:443",
		Target:       "127.0.0.1:443",
		Protocol:     "tcp",
		BindUpstream: "FN",
	}})
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestBindUpstreamUnknownFails(t *testing.T) {
	cfg := clientConfigWithForward([]ForwardRule{{
		Listen:       "0.0.0.0:443",
		Target:       "127.0.0.1:443",
		Protocol:     "tcp",
		BindUpstream: "XX",
	}})
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown bind_upstream")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bind_upstream") {
		t.Errorf("error should mention bind_upstream: %q", msg)
	}
	if !strings.Contains(msg, "XX") {
		t.Errorf("error should mention the unknown name XX: %q", msg)
	}
	if !strings.Contains(msg, "0.0.0.0:443") {
		t.Errorf("error should mention the rule's listen: %q", msg)
	}
}

func TestBindUpstreamEmptyValid(t *testing.T) {
	cfg := clientConfigWithForward([]ForwardRule{{
		Listen:   "0.0.0.0:443",
		Target:   "127.0.0.1:443",
		Protocol: "tcp",
	}})
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected empty bind_upstream to be valid, got: %v", err)
	}
}

func legacyClientConfig(bind string) *Config {
	return &Config{
		Role: "client",
		Name: "test",
		Network: NetworkConfig{
			Interface: "eth0",
			IPv4: IPv4Config{
				Addr:      "10.0.0.1:0",
				RouterMAC: "aa:bb:cc:dd:ee:ff",
			},
		},
		Server: &ServerEndpoint{Addr: "1.2.3.4:9000"},
		Forward: []ForwardRule{{
			Listen:       "0.0.0.0:443",
			Target:       "127.0.0.1:443",
			Protocol:     "tcp",
			BindUpstream: bind,
		}},
		Transport: TransportConfig{KCP: KCPConfig{Key: "K"}},
	}
}

func TestBindUpstreamLegacyDefault(t *testing.T) {
	if err := legacyClientConfig("default").Validate(); err != nil {
		t.Fatalf("expected legacy default bind to be valid, got: %v", err)
	}
	if err := legacyClientConfig("nope").Validate(); err == nil {
		t.Fatal("expected error for unknown bind_upstream in legacy form")
	}
}

func TestUpstreamNameSetFallback(t *testing.T) {
	cfg := &Config{
		Upstream: &UpstreamConfig{
			Servers: []UpstreamServer{
				{Addr: "1.2.3.4:9001", Key: "k1"},
				{Addr: "5.6.7.8:9002", Key: "k2"},
			},
		},
	}
	names := cfg.upstreamNameSet()
	if !names["upstream-1"] {
		t.Errorf("expected upstream-1 in name set: %v", names)
	}
	if !names["upstream-2"] {
		t.Errorf("expected upstream-2 in name set: %v", names)
	}
}
