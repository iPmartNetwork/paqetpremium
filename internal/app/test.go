package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/pcap"
	"github.com/paqetpremium/paqetpremium/internal/platform"
	"github.com/paqetpremium/paqetpremium/internal/tunnel"
)

type TestResult struct {
	OK     bool
	Checks []string
}

func TestConfig(path string) (*TestResult, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	res := &TestResult{OK: true}
	add := func(ok bool, msg string) {
		prefix := "[OK]"
		if !ok {
			prefix = "[FAIL]"
			res.OK = false
		}
		res.Checks = append(res.Checks, fmt.Sprintf("%s %s", prefix, msg))
	}

	add(true, fmt.Sprintf("config loaded (role=%s, name=%s)", cfg.Role, cfg.Name))

	var netRT *config.NetworkRuntime
	var nerr error
	if platform.IsLinux() {
		netRT, nerr = cfg.ResolveNetwork()
		if nerr != nil {
			add(false, nerr.Error())
		} else {
			add(true, fmt.Sprintf("network interface: %s", netRT.Interface.Name))
			add(true, fmt.Sprintf("ipv4 bind: %s", netRT.IPv4.String()))
			add(macOK(netRT.RouterMAC.String()), fmt.Sprintf("router mac: %s", netRT.RouterMAC))
			add(true, fmt.Sprintf("pcap local port: %d", netRT.Port))
		}
	} else {
		add(true, fmt.Sprintf("network interface: %s (resolve on Linux VPS)", cfg.Network.Interface))
		add(true, fmt.Sprintf("ipv4 addr: %s", cfg.Network.IPv4.Addr))
		add(macOK(cfg.Network.IPv4.RouterMAC), fmt.Sprintf("router mac: %s", cfg.Network.IPv4.RouterMAC))
	}

	add(cfg.Transport.KCP.Key != "", "transport secret key present")
	add(true, fmt.Sprintf("transport protocol: %s", cfg.Transport.Protocol))

	if cfg.Role == config.RoleServer {
		if _, err := cfg.ListenUDPAddr(); err != nil {
			add(false, err.Error())
		} else {
			add(true, fmt.Sprintf("listen: %s", cfg.Listen.Addr))
		}
	}

	if cfg.Role == config.RoleClient {
		if cfg.Upstream != nil {
			add(len(cfg.Upstream.Servers) > 0, fmt.Sprintf("upstream servers: %d", len(cfg.Upstream.Servers)))
		}
		if _, err := cfg.RemoteServerAddr(); err != nil {
			add(false, err.Error())
		} else {
			addr, _ := cfg.RemoteServerAddr()
			add(true, fmt.Sprintf("remote server: %s", addr))
		}
		add(len(cfg.Forward) > 0 || len(cfg.SOCKS5) > 0, "forward/socks5 rules configured")
	}

	if platform.IsLinux() && nerr == nil {
		if netRT.IPv6 != nil {
			add(true, fmt.Sprintf("ipv6 bind: %s", netRT.IPv6.String()))
		}
		if cfg.Admin != nil && cfg.Admin.Listen != "" {
			add(true, fmt.Sprintf("admin listen: %s", cfg.Admin.Listen))
			if cfg.Admin.Token != "" {
				add(true, "admin token configured")
			}
		}
		if err := pcap.Probe(netRT); err != nil {
			add(false, fmt.Sprintf("pcap probe: %v", err))
		} else {
			add(true, "pcap probe: interface accessible (root required at runtime)")
		}

		if cfg.Role == config.RoleClient {
			if err := tunnel.PingConfig(context.Background(), cfg); err != nil {
				add(false, fmt.Sprintf("kcp ping: %v", err))
			} else {
				add(true, "kcp ping: server reachable")
			}
		}
	} else if !platform.IsLinux() {
		res.Checks = append(res.Checks, "[SKIP] pcap/kcp live checks (Linux VPS only)")
	}

	return res, nil
}

func macOK(v string) bool {
	parts := strings.Split(v, ":")
	if len(parts) != 6 {
		return false
	}
	for _, p := range parts {
		if len(p) != 2 {
			return false
		}
	}
	return true
}
