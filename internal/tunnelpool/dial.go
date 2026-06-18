package tunnelpool

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/netutil"
	"github.com/paqetpremium/paqetpremium/internal/pcap"
	"github.com/paqetpremium/paqetpremium/internal/protocol"
	"github.com/paqetpremium/paqetpremium/internal/transport"
	"net"

	"github.com/xtaci/smux"
)

func New(ctx context.Context, cfg *config.Config, remoteFlags []netutil.TCPFlagSet) (*Pool, error) {
	remote, err := cfg.RemoteServerAddr()
	if err != nil {
		return nil, err
	}
	return Dial(ctx, cfg, remote, cfg.Transport, remoteFlags)
}

func Dial(ctx context.Context, cfg *config.Config, remote *net.UDPAddr, transportCfg config.TransportConfig, remoteFlags []netutil.TCPFlagSet) (*Pool, error) {
	opt, err := transport.OptionsFromConfig(config.RoleClient, transportCfg)
	if err != nil {
		return nil, err
	}

	count := cfg.Transport.Conn
	if count < 1 {
		count = 1
	}

	pool := &Pool{udp: make(map[uint64]*smux.Stream)}

	for i := 0; i < count; i++ {
		netRT, err := cfg.ResolveNetworkWithPort(0)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("connection %d: %w", i+1, err)
		}
		if netRT.Port == 0 {
			netRT.Port = 32768 + rand.Intn(32768)
			netRT.IPv4.Port = netRT.Port
		}

		pconn, err := pcap.Open(ctx, netRT)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("pcap %d: %w", i+1, err)
		}
		pconn.SetRemoteTCPFlags(remote, remoteFlags)

		sess, err := transport.Dial(ctx, remote, opt, pconn)
		if err != nil {
			pconn.Close()
			pool.Close()
			return nil, fmt.Errorf("%s dial %d: %w", opt.Protocol, i+1, err)
		}

		if err := pool.sendRemoteFlags(sess, remoteFlags); err != nil {
			sess.Close()
			pool.Close()
			return nil, fmt.Errorf("tcpf %d: %w", i+1, err)
		}

		pool.sessions = append(pool.sessions, sess)
	}

	return pool, nil
}

func (p *Pool) Ping() error {
	return p.PingWithTimeout(5 * time.Second)
}

func (p *Pool) PingWithTimeout(timeout time.Duration) error {
	strm, err := p.openStream()
	if err != nil {
		return err
	}
	defer strm.Close()
	_ = strm.SetDeadline(time.Now().Add(timeout))
	return protocol.PingRoundTrip(strm)
}
