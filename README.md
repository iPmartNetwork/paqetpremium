# PaqetPremium

<p align="center">
  <strong>Packet-level tunnel for Linux VPS</strong> — libpcap + KCP/QUIC + smux.<br>
  <a href="README.md">English</a> · <a href="README.fa.md">فارسی</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-linux%20amd64%20%7C%20arm64-blue" alt="platform">
  <img src="https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="go">
  <img src="https://img.shields.io/badge/transport-KCP%20%7C%20QUIC-success" alt="transport">
  <img src="https://img.shields.io/badge/version-0.9.0-informational" alt="version">
  <img src="https://img.shields.io/badge/license-GPL--3.0-blue" alt="license">
</p>

---

PaqetPremium carries traffic inside **crafted raw TCP packets** on a Linux
interface (via libpcap), then runs **KCP** or **QUIC** as the payload protocol
and **smux** for stream multiplexing. It is designed for a two-node deployment:

| Role | Location | Config `role` | Purpose |
|------|----------|---------------|---------|
| Entry | Iran VPS | `client` | TCP port-forward and SOCKS5 toward services |
| Exit | Foreign VPS (Kharej) | `server` | Relays tunnel traffic to the open internet |

> Windows, macOS, and desktop clients are **out of scope**. `run` must execute
> on Linux with root privileges (raw sockets / pcap).

## Architecture

```
   user / internet         Iran VPS (role: client)
        ───────────►   ┌───────────────────────────────┐
                       │  port-forward (TCP/UDP)        │
                       │  SOCKS5 (CONNECT + UDP ASSOC.) │
                       │            │                   │
                       │      pcap (crafted TCP)        │
                       │            │                   │
                       │   KCP / QUIC  →  smux          │
                       └──────────────┬────────────────┘
                                      │  tunnel
                       ┌──────────────▼────────────────┐
                       │  Foreign VPS (role: server)    │
                       │  relay to destination          │
                       │  iptables + ip6tables          │
                       └────────────────────────────────┘
```

## Features

- **Dual transport** — KCP (default, tuned for lossy links) or QUIC (TLS 1.3), selectable in config.
- **Mutual authentication** — both transports authenticate peers from a shared secret. QUIC pins a deterministic, secret-derived certificate on both sides.
- **Port forwarding** — TCP and UDP, per-rule upstream binding.
- **SOCKS5** — CONNECT (TCP) and UDP ASSOCIATE, with optional user/password auth.
- **Multi-upstream** — `failover`, `round_robin`, `weighted`, `least_latency`, with health checks and automatic failover.
- **Hot reload** — client (upstream + forward + SOCKS5) and server (config + firewall) via `SIGHUP` or the admin API.
- **IPv4 + optional IPv6** over the same crafted-TCP path.
- **Admin API & metrics** — health, status, reload, and Prometheus metrics, with optional bearer-token auth.
- **systemd-native** — single service or multiple named client instances.

## Requirements

- Linux (amd64 or arm64), root privileges.
- `libpcap` headers and a C toolchain (build uses **CGO**).
- `iptables` / `ip6tables` on the server node.

## Quick install

One-line bootstrap (clones, builds, and launches the guided installer):

```bash
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/paqetpremium/master/scripts/install-linux.sh | sudo bash
```

Run a specific flow directly:

```bash
# Foreign VPS (exit)
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/paqetpremium/master/scripts/install-linux.sh | sudo bash -s -- server

# Iran VPS (entry)
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/paqetpremium/master/scripts/install-linux.sh | sudo bash -s -- client
```

Or clone and run the installer/manager directly:

```bash
git clone https://github.com/iPmartNetwork/paqetpremium
cd paqetpremium
sudo ./install-premium.sh            # interactive menu
```

The installer detects your interface/IP/MAC, installs dependencies (and a recent
Go toolchain if needed), builds the binary, writes the config, and sets up
systemd — including a post-start health check.

## Manual build

```bash
sudo apt install -y libpcap-dev        # Debian/Ubuntu
make build-linux-amd64                 # or: build-linux-arm64
# manual equivalent:
CGO_ENABLED=1 go build -o paqetpremium ./cmd/paqetpremium
```

Off Linux you can still validate config (no pcap):

```bash
go build -o paqetpremium ./cmd/paqetpremium
./paqetpremium test -c example/client.yaml
```

## CLI

```bash
paqetpremium run    -c config.yaml   # run tunnel (Linux + root)
paqetpremium test   -c config.yaml   # validate config (+ live checks on Linux)
paqetpremium bench  -c client.yaml   # measure upstream latency (Linux)
paqetpremium reload -c client.yaml   # hot reload via admin API
paqetpremium version
```

## Service management

The installer ships management commands:

```bash
sudo ./install-premium.sh status            # services + admin status
sudo ./install-premium.sh logs   client     # follow logs (server|client|<tunnel>)
sudo ./install-premium.sh reload client      # SIGHUP hot reload
sudo ./install-premium.sh restart server
sudo ./install-premium.sh update             # rebuild from repo and restart
sudo ./install-premium.sh add-tunnel         # add a named client instance
sudo ./install-premium.sh uninstall
```

Equivalent systemd units: `paqetpremium-server.service`, `paqetpremium-client.service`,
and the templated `paqetpremium-client@<name>.service`.

## Configuration

Both ends must agree on the **same** `transport.protocol` and **same** secret key.

### KCP (default)

```yaml
transport:
  protocol: kcp
  conn: 6
  kcp:
    mode: fast
    block: aes-128-gcm
    key: SHARED_SECRET
    mtu: 1150
```

### QUIC

```yaml
transport:
  protocol: quic
  conn: 6
  kcp:
    key: SHARED_SECRET    # shared secret (same field as KCP)
  quic:
    alpn: paqetpremium
    idle_timeout: 30s
    max_idle_timeout: 60s
```

### Multi-upstream

```yaml
upstream:
  strategy: failover        # failover | round_robin | weighted | least_latency
  health_check:
    interval: 10s
    timeout: 3s
    fail_threshold: 3
    recover_threshold: 2
  servers:
    - name: de-fra-1
      addr: 45.1.1.1:8888
      key: SHARED_SECRET
      priority: 1
      weight: 3
    - name: nl-ams-1
      addr: 45.2.2.2:8888
      key: SHARED_SECRET
      priority: 2
```

### SOCKS5 (TCP + UDP)

```yaml
socks5:
  - listen: "127.0.0.1:1080"
    # optional:
    # auth: { user: alice, pass: secret }
```

### IPv6 (optional)

```yaml
network:
  interface: eth0
  ipv4:
    addr: "10.0.0.5:0"
    router_mac: "aa:bb:cc:dd:ee:ff"
  ipv6:
    addr: "[2001:db8::5]:0"
    router_mac: "aa:bb:cc:dd:ee:ff"
```

See `example/` for complete `client.yaml`, `server.yaml`, `client-quic.yaml`, and `server-quic.yaml`.

## Admin API

Enabled when `admin.listen` is set:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/healthz` | GET | Liveness probe |
| `/api/v1/status` | GET | JSON status (role, upstreams, sessions) |
| `/api/v1/reload` | POST | Reload config from disk |
| `/metrics` | GET | Prometheus metrics (`admin.metrics: true`) |

Set `admin.token` to protect `/api/v1/*` and `/metrics` (not `/healthz`). Pass it
as `Authorization: Bearer <token>` or `?token=<token>`.

## Security notes

- The shared secret is the trust anchor. KCP derives its block cipher key from it; QUIC pins a certificate derived deterministically from it on **both** client and server (mutual). Use a strong, unique secret and keep config files readable only by root (`0640`).
- The admin API binds to `127.0.0.1` by default. If you expose it, always set `admin.token`.
- The server applies `iptables`/`ip6tables` rules (NOTRACK + drop RST) on the tunnel port; ensure your firewall policy allows the chosen port.

## Project layout

```
cmd/paqetpremium/     CLI entrypoint
internal/
  app/                run loop, test/bench/reload helpers
  config/             YAML config and validation
  netutil/            TCP flags, address helpers
  pcap/               Linux raw packet engine (libpcap)
  transport/          KCP + QUIC + smux sessions
  tunnel/             client/server/relay runners
  tunnelpool/         multi-session pool
  upstream/           multi-server manager + health
  forward/            TCP/UDP port forwarding
  socks5/             SOCKS5 (TCP + UDP)
  iptables/           server firewall rules
  admin/              HTTP API + metrics
  metrics/            counters + Prometheus
  protocol/           tunnel control messages
  platform/           Linux deployment constraints
  version/            build metadata
example/              ready-to-edit YAML configs
install-premium.sh    installer & manager
scripts/install-linux.sh   one-line bootstrap
```

Targets: **linux/amd64**, **linux/arm64**.

## Status & roadmap

Core implementation is complete and unit-tested for the platform-independent
paths; end-to-end testing on live VPSes is the remaining step before a `1.0.0` tag.

- [x] pcap engine, KCP transport, ping handshake
- [x] port-forward, SOCKS5, session pool, iptables
- [x] multi-upstream, health checks, hot reload
- [x] admin API, metrics, IPv6, installer
- [x] reload/bench CLI, admin auth, arm64
- [x] QUIC transport with mutual certificate pinning
- [ ] live VPS validation and `1.0.0` release

See [CHANGELOG.md](CHANGELOG.md) for release notes.

## License

Released under the **GNU General Public License v3.0**. See the [LICENSE](LICENSE) file for the full text.
