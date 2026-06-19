//go:build linux

package redirect

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strconv"
	"sync"

	"github.com/paqetpremium/paqetpremium/internal/ioext"
	"github.com/paqetpremium/paqetpremium/internal/metrics"
	"github.com/paqetpremium/paqetpremium/internal/tunnelpool"
	"golang.org/x/sys/unix"
)

const (
	soOriginalDst = 80
	chainName     = "PAQET_REDIRECT"
)

// Config describes the transparent range-redirect runtime.
type Config struct {
	RedirectPort int
	TargetHost   string
	PortRanges   [][2]int
	Exclude      []int
	BindUpstream string
}

// Manager runs the transparent all-ports redirect: it installs iptables nat
// REDIRECT rules for the configured ranges and tunnels each accepted connection
// to target_host:<original destination port> recovered via SO_ORIGINAL_DST.
type Manager struct {
	route   func(string) tunnelpool.Opener
	log     *slog.Logger
	cfg     Config
	wg      sync.WaitGroup
	applied bool
}

func NewManager(route func(string) tunnelpool.Opener, log *slog.Logger) *Manager {
	return &Manager{route: route, log: log}
}

func (m *Manager) Start(ctx context.Context, cfg Config) error {
	m.cfg = cfg
	if err := m.applyRules(); err != nil {
		m.removeRules()
		return fmt.Errorf("redirect iptables: %w", err)
	}
	m.applied = true

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.RedirectPort))
	if err != nil {
		m.removeRules()
		return fmt.Errorf("redirect listen :%d: %w", cfg.RedirectPort, err)
	}
	m.log.Info("range redirect listening",
		"redirect_port", cfg.RedirectPort, "target_host", cfg.TargetHost)

	go func() {
		<-ctx.Done()
		ln.Close()
		m.removeRules()
	}()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					m.log.Warn("redirect accept", "err", err)
					continue
				}
			}
			m.wg.Add(1)
			go func(c net.Conn) {
				defer m.wg.Done()
				defer c.Close()
				m.handle(ctx, c)
			}(conn)
		}
	}()
	return nil
}

func (m *Manager) Wait() { m.wg.Wait() }

func (m *Manager) handle(ctx context.Context, c net.Conn) {
	tcp, ok := c.(*net.TCPConn)
	if !ok {
		return
	}
	_, port, err := originalDst(tcp)
	if err != nil {
		m.log.Warn("redirect original dst", "err", err)
		return
	}
	target := net.JoinHostPort(m.cfg.TargetHost, strconv.Itoa(port))

	op := m.route(m.cfg.BindUpstream)
	if op == nil {
		m.log.Warn("redirect: no tunnel opener")
		return
	}
	strm, err := op.OpenTCP(target)
	if err != nil {
		metrics.Default.IncError()
		m.log.Warn("redirect open tcp", "target", target, "err", err)
		return
	}
	defer strm.Close()

	errCh := make(chan error, 2)
	go func() { errCh <- ioext.CopyMetered(strm, c, metrics.Default, true) }()
	go func() { errCh <- ioext.CopyMetered(c, strm, metrics.Default, false) }()
	select {
	case <-ctx.Done():
	case <-errCh:
	}
}

// originalDst recovers the pre-REDIRECT destination via SO_ORIGINAL_DST.
func originalDst(c *net.TCPConn) (net.IP, int, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return nil, 0, err
	}
	var ip net.IP
	var port int
	var serr error
	cerr := raw.Control(func(fd uintptr) {
		mreq, gerr := unix.GetsockoptIPv6Mreq(int(fd), unix.SOL_IP, soOriginalDst)
		if gerr != nil {
			serr = gerr
			return
		}
		// sockaddr_in layout in Multiaddr: [2:4]=port (big endian), [4:8]=IPv4.
		port = int(mreq.Multiaddr[2])<<8 | int(mreq.Multiaddr[3])
		ip = net.IPv4(mreq.Multiaddr[4], mreq.Multiaddr[5], mreq.Multiaddr[6], mreq.Multiaddr[7])
	})
	if cerr != nil {
		return nil, 0, cerr
	}
	if serr != nil {
		return nil, 0, serr
	}
	return ip, port, nil
}

func (m *Manager) applyRules() error {
	// Fresh dedicated chain.
	_ = run("iptables", "-t", "nat", "-N", chainName)
	if err := run("iptables", "-t", "nat", "-F", chainName); err != nil {
		return err
	}
	// Exclusions first (SSH etc.), plus the redirect port itself to avoid loops.
	excl := append([]int{}, m.cfg.Exclude...)
	excl = append(excl, m.cfg.RedirectPort)
	for _, p := range excl {
		if err := run("iptables", "-t", "nat", "-A", chainName, "-p", "tcp",
			"--dport", strconv.Itoa(p), "-j", "RETURN"); err != nil {
			return err
		}
	}
	// Redirect the configured ranges.
	for _, r := range m.cfg.PortRanges {
		spec := strconv.Itoa(r[0])
		if r[1] != r[0] {
			spec = fmt.Sprintf("%d:%d", r[0], r[1])
		}
		if err := run("iptables", "-t", "nat", "-A", chainName, "-p", "tcp",
			"--dport", spec, "-j", "REDIRECT", "--to-ports", strconv.Itoa(m.cfg.RedirectPort)); err != nil {
			return err
		}
	}
	// Hook into PREROUTING (idempotent: delete any prior, then add).
	_ = run("iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp", "-j", chainName)
	if err := run("iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp", "-j", chainName); err != nil {
		return err
	}
	return nil
}

func (m *Manager) removeRules() {
	if !m.applied {
		return
	}
	_ = run("iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp", "-j", chainName)
	_ = run("iptables", "-t", "nat", "-F", chainName)
	_ = run("iptables", "-t", "nat", "-X", chainName)
	m.applied = false
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %v (%s)", name, args, err, string(out))
	}
	return nil
}
