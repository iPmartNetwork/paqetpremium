package tunnel

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/paqetpremium/paqetpremium/internal/admin"
	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/forward"
	"github.com/paqetpremium/paqetpremium/internal/metrics"
	"github.com/paqetpremium/paqetpremium/internal/redirect"
	"github.com/paqetpremium/paqetpremium/internal/socks5"
	"github.com/paqetpremium/paqetpremium/internal/tunnelpool"
	"github.com/paqetpremium/paqetpremium/internal/upstream"
	"github.com/paqetpremium/paqetpremium/internal/version"
)

type Client struct {
	cfgPath string
	cfg     atomic.Pointer[config.Config]
	log     *slog.Logger

	mu       sync.RWMutex
	upstream *upstream.Manager

	runCtx context.Context

	svcMu     sync.Mutex
	svcCancel context.CancelFunc
	svcWg     sync.WaitGroup
}

func NewClient(cfg *config.Config, cfgPath string, log *slog.Logger) *Client {
	c := &Client{cfgPath: cfgPath, log: log}
	c.cfg.Store(cfg)
	return c
}

func (c *Client) Run(ctx context.Context) error {
	cfg := c.cfg.Load()
	netRT, err := cfg.ResolveNetwork()
	if err != nil {
		return err
	}

	up, err := upstream.NewManager(ctx, cfg, netRT.RemoteFlags, c.log)
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	c.mu.Lock()
	c.upstream = up
	c.mu.Unlock()
	defer up.Close()

	c.log.Info("tunnel ready",
		"strategy", cfg.UpstreamStrategy(),
		"active_upstream", up.ActiveName(),
		"servers", len(cfg.UpstreamNames()),
	)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.runCtx = runCtx
	defer c.stopServices()

	go c.watchReload(runCtx)
	c.startAdmin(runCtx, up)

	c.startServices(runCtx, cfg)

	<-runCtx.Done()
	c.log.Info("client shutting down")
	return nil
}

func (c *Client) startServices(ctx context.Context, cfg *config.Config) {
	c.stopServices()

	svcCtx, cancel := context.WithCancel(ctx)
	c.svcMu.Lock()
	c.svcCancel = cancel
	c.svcMu.Unlock()

	route := c.routeFn()

	if len(cfg.Forward) > 0 {
		fwd := forward.NewManager(route, c.log)
		rules := forwardRulesFromConfig(cfg)
		if err := fwd.Start(svcCtx, rules); err != nil {
			c.log.Error("forward start failed", "err", err)
			return
		}
		c.svcWg.Add(1)
		go func() {
			defer c.svcWg.Done()
			fwd.Wait()
		}()
	}

	if len(cfg.SOCKS5) > 0 {
		socks := socks5.New(route, c.log)
		c.svcWg.Add(1)
		go func(rules []config.SOCKS5Rule) {
			defer c.svcWg.Done()
			_ = socks.Start(svcCtx, rules)
		}(cfg.SOCKS5)
	}

	if cfg.Range != nil && cfg.Range.Enabled {
		ranges, _ := cfg.Range.PortRanges()
		excl, _ := cfg.Range.ExcludePorts()
		rm := redirect.NewManager(route, c.log)
		if err := rm.Start(svcCtx, redirect.Config{
			RedirectPort: cfg.Range.RedirectPort,
			TargetHost:   cfg.Range.TargetHost,
			PortRanges:   ranges,
			Exclude:      excl,
			BindUpstream: cfg.Range.BindUpstream,
		}); err != nil {
			c.log.Error("range redirect start failed", "err", err)
		} else {
			c.svcWg.Add(1)
			go func() {
				defer c.svcWg.Done()
				rm.Wait()
			}()
		}
	}

	c.log.Info("client services started",
		"forward", len(cfg.Forward),
		"socks5", len(cfg.SOCKS5),
		"range", cfg.Range != nil && cfg.Range.Enabled,
	)
}

func (c *Client) stopServices() {
	c.svcMu.Lock()
	cancel := c.svcCancel
	c.svcCancel = nil
	c.svcMu.Unlock()
	if cancel != nil {
		cancel()
		c.svcWg.Wait()
	}
}

func forwardRulesFromConfig(cfg *config.Config) []forward.Rule {
	rules := make([]forward.Rule, 0, len(cfg.Forward))
	for _, r := range cfg.Forward {
		proto := r.Protocol
		if proto == "" {
			proto = "tcp"
		}
		rules = append(rules, forward.Rule{
			Listen:       r.Listen,
			Target:       r.Target,
			Proto:        proto,
			BindUpstream: r.BindUpstream,
		})
	}
	return rules
}

func (c *Client) routeFn() forward.RouteFn {
	return func(bind string) tunnelpool.Opener {
		c.mu.RLock()
		up := c.upstream
		c.mu.RUnlock()
		if up == nil {
			return nil
		}
		if bind == "" {
			return up
		}
		return up.ForServer(bind)
	}
}

func (c *Client) watchReload(ctx context.Context) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			if err := c.ReloadFromDisk(ctx); err != nil {
				c.log.Error("reload failed", "err", err)
			}
		}
	}
}

func (c *Client) ReloadFromDisk(ctx context.Context) error {
	if c.cfgPath == "" {
		return fmt.Errorf("config path unknown")
	}
	cfg, err := config.Load(c.cfgPath)
	if err != nil {
		return err
	}
	netRT, err := cfg.ResolveNetwork()
	if err != nil {
		return err
	}

	c.mu.RLock()
	up := c.upstream
	c.mu.RUnlock()
	if up == nil {
		return fmt.Errorf("upstream not ready")
	}

	if err := up.Reload(ctx, cfg, netRT.RemoteFlags); err != nil {
		return err
	}
	c.cfg.Store(cfg)

	parent := c.runCtx
	if parent == nil {
		parent = ctx
	}
	c.startServices(parent, cfg)

	c.log.Info("config reloaded",
		"file", c.cfgPath,
		"forward", len(cfg.Forward),
		"socks5", len(cfg.SOCKS5),
	)
	return nil
}

func (c *Client) startAdmin(ctx context.Context, up *upstream.Manager) {
	cfg := c.cfg.Load()
	if cfg.Admin == nil {
		return
	}
	srv := admin.New(cfg.Admin, c.log, func() admin.Status {
		cfg := c.cfg.Load()
		snap := up.Snapshot()
		sessions := 0
		for _, s := range snap {
			sessions += s.Sessions
		}
		stats := metrics.Default.Snapshot()
		return admin.Status{
			Core:      version.Name,
			Version:   version.Version,
			Role:      "client",
			Name:      cfg.Name,
			Strategy:  up.Strategy(),
			Active:    up.ActiveName(),
			Sessions:  sessions,
			Upstreams: snap,
			Stats:     &stats,
		}
	}, func() error {
		return c.ReloadFromDisk(ctx)
	})
	srv.WithConfigEditor(c.cfgPath, func() *config.Config { return c.cfg.Load() })
	go func() {
		if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
			c.log.Error("admin server failed", "err", err)
		}
	}()
}

// PingConfig validates config and pings all upstream servers.
func PingConfig(ctx context.Context, cfg *config.Config) error {
	netRT, err := cfg.ResolveNetwork()
	if err != nil {
		return err
	}
	up, err := upstream.NewManager(ctx, cfg, netRT.RemoteFlags, slog.Default())
	if err != nil {
		return err
	}
	defer up.Close()
	return up.PingAll()
}

// BenchConfig measures KCP ping latency to each upstream server.
func BenchConfig(ctx context.Context, cfg *config.Config) ([]upstream.BenchResult, error) {
	netRT, err := cfg.ResolveNetwork()
	if err != nil {
		return nil, err
	}
	timeout := cfg.UpstreamHealthTimeout()
	up, err := upstream.NewManager(ctx, cfg, netRT.RemoteFlags, slog.Default())
	if err != nil {
		return nil, err
	}
	defer up.Close()
	return up.Benchmark(ctx, timeout), nil
}
