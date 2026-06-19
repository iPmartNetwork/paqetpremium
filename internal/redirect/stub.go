//go:build !linux

package redirect

import (
	"context"
	"errors"
	"log/slog"

	"github.com/paqetpremium/paqetpremium/internal/tunnelpool"
)

// Config mirrors the linux build so non-linux compiles.
type Config struct {
	RedirectPort int
	TargetHost   string
	PortRanges   [][2]int
	Exclude      []int
	BindUpstream string
}

type Manager struct{}

func NewManager(route func(string) tunnelpool.Opener, log *slog.Logger) *Manager { return &Manager{} }

func (m *Manager) Start(ctx context.Context, cfg Config) error {
	return errors.New("range redirect mode is only supported on Linux")
}

func (m *Manager) Wait() {}
