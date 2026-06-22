package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

const rangeOnlyClientConfig = `role: client
network:
  interface: eth0
  ipv4:
    addr: "10.0.0.1:0"
    router_mac: "aa:bb:cc:dd:ee:ff"
transport:
  protocol: kcp
  kcp:
    key: "K"
range:
  enabled: true
  ports: "1-65535"
  target_host: "127.0.0.1"
upstream:
  servers:
    - addr: "203.0.113.1:9000"
      key: "S"
`

func TestConfigRangeOnlyClient(t *testing.T) {
	path := writeTempConfig(t, rangeOnlyClientConfig)
	res, err := TestConfig(path)
	if err != nil {
		t.Fatalf("TestConfig returned error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected res.OK == true, got false; checks=%v", res.Checks)
	}

	var hasRulesLine, hasRangeMode bool
	for _, c := range res.Checks {
		if strings.HasPrefix(c, "[FAIL]") {
			t.Errorf("unexpected failing check: %s", c)
		}
		if strings.HasPrefix(c, "[OK] forward/socks5/range rules configured") {
			hasRulesLine = true
		}
		if strings.Contains(c, "range mode:") {
			hasRangeMode = true
		}
	}
	if !hasRulesLine {
		t.Errorf("missing [OK] forward/socks5/range rules configured line; checks=%v", res.Checks)
	}
	if !hasRangeMode {
		t.Errorf("missing range mode: line; checks=%v", res.Checks)
	}
}

const noRulesClientConfig = `role: client
network:
  interface: eth0
  ipv4:
    addr: "10.0.0.1:0"
    router_mac: "aa:bb:cc:dd:ee:ff"
transport:
  protocol: kcp
  kcp:
    key: "K"
upstream:
  servers:
    - addr: "203.0.113.1:9000"
      key: "S"
`

func TestConfigNoRulesClientRejected(t *testing.T) {
	path := writeTempConfig(t, noRulesClientConfig)
	res, err := TestConfig(path)
	if err == nil {
		t.Fatalf("expected error for client with no forward/socks5/range rules, got nil (res=%v)", res)
	}
}
