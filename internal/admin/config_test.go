package admin

import (
	"testing"

	"github.com/paqetpremium/paqetpremium/internal/config"
)

func sampleConfig() *config.Config {
	return &config.Config{
		Role: config.RoleClient,
		Network: config.NetworkConfig{
			Interface: "eth0",
			IPv4:      config.IPv4Config{Addr: "10.0.0.1", RouterMAC: "aa:bb:cc:dd:ee:ff"},
		},
		Transport: config.TransportConfig{
			Protocol: "kcp",
			KCP:      config.KCPConfig{Key: "supersecret"},
		},
		Upstream: &config.UpstreamConfig{
			Servers: []config.UpstreamServer{{Name: "kharej-1", Addr: "1.2.3.4:22490", Key: "up-secret"}},
		},
		Forward: []config.ForwardRule{{Listen: ":8080", Target: "127.0.0.1:80", Protocol: "tcp"}},
	}
}

func TestRedactConfigHidesSecrets(t *testing.T) {
	red, err := redactConfig(sampleConfig())
	if err != nil {
		t.Fatalf("redact: %v", err)
	}
	if red.Transport.KCP.Key != redactedMark {
		t.Errorf("kcp key not redacted: %q", red.Transport.KCP.Key)
	}
	if red.Upstream.Servers[0].Key != redactedMark {
		t.Errorf("upstream key not redacted: %q", red.Upstream.Servers[0].Key)
	}
}

func TestRedactDoesNotMutateOriginal(t *testing.T) {
	orig := sampleConfig()
	if _, err := redactConfig(orig); err != nil {
		t.Fatal(err)
	}
	if orig.Transport.KCP.Key != "supersecret" {
		t.Errorf("original mutated: %q", orig.Transport.KCP.Key)
	}
}

func TestRestoreSecretsKeepsRunningValues(t *testing.T) {
	cur := sampleConfig()
	incoming := sampleConfig()
	incoming.Transport.KCP.Key = redactedMark
	incoming.Upstream.Servers[0].Key = redactedMark
	restoreSecrets(incoming, cur)
	if incoming.Transport.KCP.Key != "supersecret" {
		t.Errorf("kcp key not restored: %q", incoming.Transport.KCP.Key)
	}
	if incoming.Upstream.Servers[0].Key != "up-secret" {
		t.Errorf("upstream key not restored: %q", incoming.Upstream.Servers[0].Key)
	}
}

func TestRestoreSecretsAllowsExplicitChange(t *testing.T) {
	cur := sampleConfig()
	incoming := sampleConfig()
	incoming.Transport.KCP.Key = "newsecret"
	restoreSecrets(incoming, cur)
	if incoming.Transport.KCP.Key != "newsecret" {
		t.Errorf("explicit key change lost: %q", incoming.Transport.KCP.Key)
	}
}

func TestRoundTripValidates(t *testing.T) {
	red, err := redactConfig(sampleConfig())
	if err != nil {
		t.Fatal(err)
	}
	restoreSecrets(red, sampleConfig())
	if err := red.Validate(); err != nil {
		t.Errorf("validate after round-trip: %v", err)
	}
}
