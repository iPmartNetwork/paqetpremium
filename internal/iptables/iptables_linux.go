//go:build linux

package iptables

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/paqetpremium/paqetpremium/internal/config"
)

const comment = "paqetpremium"

// Apply installs NOTRACK + RST DROP rules for the tunnel TCP port(s).
func Apply(net *config.NetworkRuntime) error {
	if net == nil {
		return fmt.Errorf("network runtime missing")
	}
	if err := applyFamily("iptables", net.Port); err != nil {
		return err
	}
	if net.IPv6 != nil {
		port := net.IPv6.Port
		if port == 0 {
			port = net.Port
		}
		if err := applyFamily("ip6tables", port); err != nil {
			return err
		}
	}
	return nil
}

// ApplyTCPPort applies IPv4 rules only (legacy helper).
func ApplyTCPPort(port int) error {
	return applyFamily("iptables", port)
}

func applyFamily(bin string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s not found: %w", bin, err)
	}
	p := strconv.Itoa(port)
	rules := []struct {
		table string
		chain string
		args  []string
	}{
		{"raw", "PREROUTING", []string{"-p", "tcp", "--dport", p, "-m", "comment", "--comment", comment, "-j", "NOTRACK"}},
		{"raw", "OUTPUT", []string{"-p", "tcp", "--sport", p, "-m", "comment", "--comment", comment, "-j", "NOTRACK"}},
		{"mangle", "OUTPUT", []string{"-p", "tcp", "--sport", p, "--tcp-flags", "RST", "RST", "-m", "comment", "--comment", comment, "-j", "DROP"}},
	}
	for _, r := range rules {
		if err := ensureRule(bin, r.table, r.chain, r.args); err != nil {
			return err
		}
	}
	return nil
}

func Remove() error {
	for _, bin := range []string{"iptables", "ip6tables"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		removeCommented(bin)
	}
	return nil
}

func removeCommented(bin string) {
	for _, table := range []string{"raw", "mangle"} {
		out, err := exec.Command(bin, "-t", table, "-S").Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "-A ") || !strings.Contains(line, comment) {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			args := []string{"-t", table, "-D", parts[1]}
			args = append(args, parts[2:]...)
			_ = exec.Command(bin, args...).Run()
		}
	}
}

func ensureRule(bin, table, chain string, rule []string) error {
	check := append([]string{"-t", table, "-C", chain}, rule...)
	if err := exec.Command(bin, check...).Run(); err == nil {
		return nil
	}
	add := append([]string{"-t", table, "-A", chain}, rule...)
	if out, err := exec.Command(bin, add...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s: %w", bin, strings.TrimSpace(string(out)), err)
	}
	return nil
}
