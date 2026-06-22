# PaqetPremium

<p align="center">
  <strong>Packet-level tunnel for Linux VPS</strong> — libpcap + KCP/QUIC + smux.<br>
  <a href="README.md">English</a> · <a href="README.fa.md">فارسی</a>
</p>

<p align="center"><a href="https://ipmartnetwork.github.io/paqetpremium/"><strong>🌐 Website &amp; docs</strong></a></p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-linux%20amd64%20%7C%20arm64-blue" alt="platform">
  <img src="https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="go">
  <img src="https://img.shields.io/badge/transport-KCP%20%7C%20QUIC-success" alt="transport">
  <img src="https://img.shields.io/badge/version-0.18.0-informational" alt="version">
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
- **Port forwarding** — TCP and UDP, per-rule upstream binding. UDP-based protocols (QUIC, Hysteria2, TUIC, WireGuard) work over UDP forwarding — datagram boundaries are preserved end-to-end.
- **Transparent all-ports range mode** — a single client listener transparently tunnels an entire TCP port range (via iptables REDIRECT + `SO_ORIGINAL_DST`) to the server's localhost, with no per-port config and no server-side change.
- **SOCKS5** — CONNECT (TCP) and UDP ASSOCIATE, with optional user/password auth.
- **Multi-upstream** — `failover`, `round_robin`, `weighted`, `least_latency`, with health checks and automatic failover.
- **Self-healing upstreams** — dead connection pools (server restart, network blip, keepalive timeout) are rebuilt out-of-band with backoff and marked healthy again once a ping succeeds — no client restart needed.
- **Configurable KCP FEC** — optional forward error correction (`data_shard`/`parity_shard`) recovers lost packets without retransmits on lossy links; windows are tunable too.
- **Hot reload** — client (upstream + forward + SOCKS5) and server (config + firewall) via `SIGHUP` or the admin API.
- **IPv4 + optional IPv6** over the same crafted-TCP path.
- **Admin API, metrics & web dashboard** — health, status, reload, Prometheus metrics, and a live dark-theme status page, with optional bearer-token auth.
- **systemd-native** — single service or multiple named client instances, with per-tunnel list/edit/remove management.

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

### Install from a package

Tagged releases also publish `.deb` and `.rpm` packages for **amd64** and
**arm64** (the package declares a `libpcap` dependency):

```bash
sudo dpkg -i paqetpremium_*.deb      # Debian/Ubuntu
sudo rpm   -i paqetpremium-*.rpm     # RHEL/Fedora
```

Then run the installer for guided setup (`sudo ./install-premium.sh`) or start
directly with `paqetpremium run -c <config>`. The one-line bootstrap above
remains the primary install method.

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
sudo ./install-premium.sh tunnels            # list configured tunnels with details
sudo ./install-premium.sh edit   client      # edit a tunnel's config and restart it (server|client|<name>)
sudo ./install-premium.sh remove mytunnel    # delete a single tunnel (config + service)
sudo ./install-premium.sh uninstall
```

Equivalent systemd units: `paqetpremium-server.service`, `paqetpremium-client.service`,
and the templated `paqetpremium-client@<name>.service`.

`tunnels`, `edit`, and `remove` are also available as menu items. `tunnels`
lists every configured tunnel with its role, transport, upstream/forward/socks/range
summary and live status; `edit` opens one tunnel's config, validates it, and
restarts just that service; `remove` deletes a single tunnel's config and service
without touching the other tunnels or the binary.

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
    # optional forward error correction (off by default):
    data_shard: 10      # FEC: recover lost packets without retransmit (default 0 = off)
    parity_shard: 3     # both ends MUST match
    snd_wnd: 1024       # optional window overrides
    rcv_wnd: 1024
```

FEC trades a little extra bandwidth for far fewer retransmits on lossy links
(common on Iran↔abroad paths): a `data_shard: 10` / `parity_shard: 3` group
recovers up to 3 lost packets without a round-trip. It is **off by default**,
and both ends must use the **same** `data_shard`/`parity_shard`. `snd_wnd`/`rcv_wnd`
are optional window overrides; leave them unset to keep the role-based defaults.

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

With load-balancing strategies (`round_robin`, `weighted`, `least_latency`) the
client spreads connections across **all** healthy servers, so every exit must be
able to serve the traffic it receives. Use `failover` (a single active exit, with
the rest as standby) when only one server should carry traffic at a time — this is
also the right choice for range mode unless every exit exposes the same target
service (see the range-mode note below).

### Per-upstream transport

Each `upstream.servers[]` entry may include an optional `transport:` block that
**overrides** the global `transport` for that upstream only. Fields you leave out
are inherited from the global block (zero-value inheritance), so a per-upstream
block stays small — set just the protocol (or whatever differs) and the rest comes
from the global defaults. The per-server `key` is used as that upstream's secret.

This lets a single Iran client reach different exits over different transports —
for example one over KCP and another over QUIC — to adapt to different network
paths and DPI conditions.

```yaml
transport:            # global defaults (still required)
  protocol: kcp
  conn: 6
  kcp:
    key: SHARED_SECRET
    mtu: 1150
upstream:
  strategy: failover
  servers:
    - name: kharej-1   # inherits global kcp
      addr: 1.2.3.4:20201
      key: SHARED_SECRET
      priority: 1
    - name: kharej-2   # this one uses quic
      addr: 5.6.7.8:20202
      key: SHARED_SECRET
      priority: 2
      transport:
        protocol: quic
```

> Each exit server still runs its **own single transport**; the client must match
> each upstream's protocol to the server it points at. `paqetpremium test` reports
> the resolved transport protocol per upstream so you can confirm the mapping.

### SOCKS5 (TCP + UDP)

```yaml
socks5:
  - listen: "127.0.0.1:1080"
    # optional:
    # auth: { user: alice, pass: secret }
```

### Transparent all-ports range mode (client)

```yaml
range:
  enabled: true
  protocol: tcp           # tcp only (UDP transparent mode is planned)
  redirect_port: 47999    # local listener that iptables REDIRECTs into
  target_host: "127.0.0.1" # server-side host that services live on
  ports: "1-65535"        # range/list to tunnel, e.g. "443,8443,2000-3000"
  exclude: "22"           # never redirect these (keep SSH!)
```

With range mode enabled, any TCP connection to the client's IP on a port within
`ports` is transparently tunneled to `target_host:<original-port>` on the server.
The client installs an `iptables` nat REDIRECT into a single local listener and
recovers each connection's original destination port via `SO_ORIGINAL_DST`, so
you reach **any** port on the server's localhost through the entry IP without
per-port configuration — and the server needs no change (its relay already dials
the per-connection target). SSH (`22`) and the `redirect_port` are auto-excluded.
The installer offers this as the **"Tunnel ALL inbound ports transparently"**
wizard option.

> **Security:** this exposes every localhost port on the server through the entry
> IP. Keep sensitive ports (databases, the admin API, etc.) in `exclude`.

> **Strategy with range mode:** prefer `failover` (a single active exit) unless
> **every** exit server exposes the **same** target service on the configured
> `target_host`. Load-balancing strategies (`round_robin`, `weighted`,
> `least_latency`) spread connections across servers, so any server that is
> missing the target service will drop those connections.

### Multi-server port forwarding (per-exit)

When your exit servers host **different** inbounds (for example a panel on one
server and a node on another, each with its own ports), map each port to its
exit with `forward` rules and `bind_upstream`. The installer client wizard asks
each upstream's TCP ports, then (y/n) whether it also has UDP ports and which.
It then emits one bound `forward` rule per port. This is genuinely multi-server:
each port routes to its own exit, not a single active one.

```yaml
upstream:
  servers:
    - { name: DE, addr: 116.203.19.246:22490, key: SECRET, priority: 1 }
    - { name: FN, addr: 157.180.65.244:22491, key: SECRET, priority: 2 }
forward:
  - { listen: "0.0.0.0:443",  target: "127.0.0.1:443",  protocol: tcp, bind_upstream: DE }
  - { listen: "0.0.0.0:2050", target: "127.0.0.1:2050", protocol: tcp, bind_upstream: DE }
  - { listen: "0.0.0.0:443",  target: "127.0.0.1:443",  protocol: udp, bind_upstream: DE }
  - { listen: "0.0.0.0:8443", target: "127.0.0.1:8443", protocol: tcp, bind_upstream: FN }
```

Key points:

- **Ports are preserved end-to-end** — the Iran client listens on `:PORT` and the
  exit relay dials `127.0.0.1:PORT`. End users change **only** the server address
  (to the Iran IP), never the port.
- **Each exit's ports must be distinct** — routing is by destination port, so two
  exits cannot share the same port number.
- **No exit-server change is needed** — the exit's panel/inbound just listens on
  the target host (`127.0.0.1` or `0.0.0.0`); the server relay already dials the
  target the client sends.
- **`bind_upstream` must match an `upstream.servers[].name`** — otherwise config
  validation fails fast with a clear error.

### UDP protocols (WireGuard, Hysteria2)

By default a UDP forward rule rides the same reliable, ordered **smux** path as
TCP. That is the wrong shape for UDP-based protocols: a single reliable ordered
stream imposes **head-of-line blocking** and retransmission that UDP neither
expects nor wants. WireGuard degrades, and QUIC-based protocols (Hysteria2,
TUIC) effectively collapse — they are running QUIC on top of a reliable stream.

**Fix: bind the UDP inbound's upstream to a QUIC transport.** When the bound
upstream's resolved transport is `quic`, PaqetPremium carries each UDP flow as
**unreliable QUIC datagrams** ([RFC 9221](https://www.rfc-editor.org/rfc/rfc9221))
— one datagram per UDP packet, demultiplexed per flow, with **no** reliability
or ordering imposed. That is exactly the transport semantics these protocols
want, so head-of-line blocking disappears.

```yaml
upstream:
  servers:
    - name: WG
      addr: 5.6.7.8:22490
      key: SECRET
      priority: 1
      transport:
        protocol: quic       # UDP inbound -> bind it to a QUIC upstream
forward:
  - { listen: "0.0.0.0:51820", target: "127.0.0.1:51820", protocol: udp, bind_upstream: WG }
```

> **MTU guidance.** A single UDP packet must fit inside one QUIC datagram (no
> fragmentation in this version). Set the **inner** protocol's MTU low enough to
> fit the path — for WireGuard, an MTU around **1100 or lower** is a safe
> starting point. Oversized datagrams are **dropped** (counted as
> `udp_dgram_dropped`), so lower the inner MTU if you see drops.

> **KCP upstreams** keep the reliable smux UDP relay. That is fine for simple,
> low-rate UDP (e.g. DNS) but **not** for QUIC-based protocols — bind those to a
> QUIC upstream as above.

Watch datagram activity on the dashboard's **"UDP dgram"** card, or via the
`paqetpremium_udp_dgram_*` Prometheus metrics (`flows`, `in`, `out`, `dropped`).

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

### Dashboard

The admin server also serves a self-contained, dark-theme **live status page** at
its root (`/`): download/upload throughput with per-second rates, active sessions,
TCP/UDP and relay counters, error count, and a per-upstream health/RTT/sessions
table, auto-refreshing every couple of seconds. Because admin binds to
`127.0.0.1` by default, view it through an SSH tunnel:

```bash
ssh -L 9090:127.0.0.1:9090 root@<server>
# then open http://localhost:9090  (append ?token=... if an admin token is set)
```

## Security notes

- The shared secret is the trust anchor. KCP derives its block cipher key from it; QUIC pins a certificate derived deterministically from it on **both** client and server (mutual). Use a strong, unique secret and keep config files readable only by root (`0640`).
- The admin API binds to `127.0.0.1` by default. If you expose it, always set `admin.token`.
- The server applies `iptables`/`ip6tables` rules (NOTRACK + drop RST) on the tunnel port; ensure your firewall policy allows the chosen port.

### Connection reset (RST) suppression

On stateful or DPI-filtered paths (common between Iran and abroad), a middlebox
can inject or trigger a kernel TCP **RST** on the fake-TCP tunnel port and tear
the flow down. To prevent that, the installer adds an idempotent `iptables`
`OUTPUT` rule (tagged with the comment `paqetpremium`) that **drops the kernel's
TCP RST** on the tunnel port. The rule is tied to each service via systemd
`ExecStartPre=` / `ExecStopPost=`, so it is created on start and removed on stop
(and swept on uninstall). pcap still captures normally — it taps at `AF_PACKET`,
before netfilter — so only the kernel's stray RST is suppressed.

If you installed a **prebuilt binary** without the installer, add the rule
manually:

```bash
# Server (drop RST sourced from the tunnel listen port)
iptables -A OUTPUT -p tcp --sport <listen_port> --tcp-flags RST RST -j DROP

# Client (drop RST destined to each upstream port — one rule per upstream port)
iptables -A OUTPUT -p tcp --dport <upstream_port> --tcp-flags RST RST -j DROP
```

Mirror with `ip6tables` when you use IPv6.

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

Core implementation is complete, unit-tested, and exercised by an end-to-end
integration suite in CI; real-world burn-in on live VPSes is the remaining step
before a stable `1.0.0` tag.

- [x] pcap engine, KCP transport, ping handshake
- [x] port-forward, SOCKS5, session pool, iptables
- [x] multi-upstream, health checks, hot reload
- [x] admin API, metrics, IPv6, installer
- [x] reload/bench CLI, admin auth, arm64
- [x] QUIC transport with mutual certificate pinning
- [x] UDP datagram framing (boundary-preserving relay)
- [x] transparent TCP "all ports" range mode
- [x] configurable KCP FEC and windows
- [x] self-healing upstream reconnect
- [x] web dashboard
- [x] `.deb` / `.rpm` packages
- [x] per-tunnel management (list/edit/remove)
- [x] CI integration tests
- [ ] transparent UDP (TPROXY) range mode
- [ ] real-world burn-in and stable `1.0.0` release

See [CHANGELOG.md](CHANGELOG.md) for release notes.

## License

Released under the **GNU General Public License v3.0**. See the [LICENSE](LICENSE) file for the full text.
