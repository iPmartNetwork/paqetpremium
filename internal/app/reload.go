package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/platform"
	"github.com/paqetpremium/paqetpremium/internal/tunnel"
)

func ReloadConfig(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if cfg.Admin == nil || cfg.Admin.Listen == "" {
		return fmt.Errorf("admin.listen not set in config; use: kill -HUP $(pidof paqetpremium)")
	}

	url := fmt.Sprintf("http://%s/api/v1/reload", cfg.Admin.Listen)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	if cfg.Admin.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Admin.Token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reload request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reload failed (%d): %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}

type BenchLine struct {
	Name string
	Addr string
	RTT  time.Duration
	OK   bool
	Err  string
}

func BenchConfig(path string) ([]BenchLine, error) {
	if err := platform.RequireLinux(); err != nil {
		return nil, err
	}

	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if cfg.Role != config.RoleClient {
		return nil, fmt.Errorf("bench is only supported for client role")
	}

	results, err := tunnel.BenchConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	lines := make([]BenchLine, 0, len(results))
	for _, r := range results {
		lines = append(lines, BenchLine{
			Name: r.Name,
			Addr: r.Addr,
			RTT:  r.RTT,
			OK:   r.OK,
			Err:  r.Error,
		})
	}
	return lines, nil
}
