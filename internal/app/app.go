package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/platform"
	"github.com/paqetpremium/paqetpremium/internal/tunnel"
	"github.com/paqetpremium/paqetpremium/internal/version"
)

type App struct {
	cfg *config.Config
	log *slog.Logger
}

func New(cfg *config.Config) *App {
	level := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	return &App{cfg: cfg, log: logger}
}

func (a *App) Run(ctx context.Context, cfgPath string) error {
	if err := platform.RequireLinux(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	a.log.Info("starting",
		"core", version.Name,
		"version", version.Version,
		"role", a.cfg.Role,
		"name", a.cfg.Name,
	)

	switch a.cfg.Role {
	case config.RoleServer:
		return tunnel.NewServer(a.cfg, cfgPath, a.log).Run(ctx)
	case config.RoleClient:
		return tunnel.NewClient(a.cfg, cfgPath, a.log).Run(ctx)
	default:
		return fmt.Errorf("unsupported role: %s", a.cfg.Role)
	}
}

func RunConfig(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	return New(cfg).Run(context.Background(), path)
}

func RunConfigWithContext(ctx context.Context, path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	return New(cfg).Run(ctx, path)
}
