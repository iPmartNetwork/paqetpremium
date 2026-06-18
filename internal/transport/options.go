package transport

import (
	"fmt"
	"strings"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/xtaci/kcp-go/v5"
)

const (
	ProtocolKCP  = "kcp"
	ProtocolQUIC = "quic"
)

type Options struct {
	Protocol  string
	SecretKey string
	KCP       KCPOptions
	QUIC      QUICOptions
	SmuxBuf   int
	StreamBuf int
}

type KCPOptions struct {
	Block  kcp.BlockCrypt
	Mode   string
	MTU    int
	SndWnd int
	RcvWnd int
}

type QUICOptions struct {
	ALPN           string
	IdleTimeout    time.Duration
	MaxIdleTimeout time.Duration
}

func OptionsFromConfig(role string, t config.TransportConfig) (Options, error) {
	proto := strings.ToLower(strings.TrimSpace(t.Protocol))
	if proto == "" {
		proto = ProtocolKCP
	}

	key := strings.TrimSpace(t.KCP.Key)
	if key == "" {
		return Options{}, fmt.Errorf("transport.kcp.key is required (shared tunnel secret)")
	}

	snd, rcv := 512, 512
	if role == config.RoleServer {
		snd, rcv = 1024, 1024
	}

	opt := Options{
		Protocol:  proto,
		SecretKey: key,
		SmuxBuf:   4 * 1024 * 1024,
		StreamBuf: 2 * 1024 * 1024,
		QUIC: QUICOptions{
			ALPN:           strings.TrimSpace(t.QUIC.ALPN),
			IdleTimeout:    parseDuration(t.QUIC.IdleTimeout, 30*time.Second),
			MaxIdleTimeout: parseDuration(t.QUIC.MaxIdleTimeout, 60*time.Second),
		},
	}

	if opt.QUIC.ALPN == "" {
		opt.QUIC.ALPN = "paqetpremium"
	}

	switch proto {
	case ProtocolKCP:
		block, err := NewBlockCrypt(t.KCP.Block, key)
		if err != nil {
			return Options{}, err
		}
		mode := strings.TrimSpace(t.KCP.Mode)
		if mode == "" {
			mode = "fast"
		}
		mtu := t.KCP.MTU
		if mtu <= 0 {
			mtu = 1150
		}
		opt.KCP = KCPOptions{
			Block:  block,
			Mode:   mode,
			MTU:    mtu,
			SndWnd: snd,
			RcvWnd: rcv,
		}
	case ProtocolQUIC:
		// QUIC uses TLS derived from the same shared secret key.
	default:
		return Options{}, fmt.Errorf("transport.protocol: unsupported %q (use kcp or quic)", proto)
	}

	return opt, nil
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
