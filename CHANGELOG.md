# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Community health files: Contributor Covenant Code of Conduct, Contributing guide, Security policy, issue forms (bug/feature) with a chooser config, and a pull-request template.
- Project landing page (GitHub Pages) at `docs/index.html`: a self-contained, bilingual (English/Persian, RTL) dark-theme site with an architecture diagram (Mermaid), feature overview, quick-start, configuration examples with copy buttons, strategy table, dashboard/management, and FAQ. Enable via repo Settings -> Pages -> Source: master /docs.
- Installer per-tunnel management: `list tunnels` (detailed view with role, transport, upstream/forward/socks/range summary and live status), `edit [name]` (open a tunnel's config in $EDITOR, validate, and restart just that service), and `remove [name]` (delete a single tunnel's config + service without touching other tunnels or the binary). Available as menu items 4/5/6 and as `tunnels` / `edit` / `remove` subcommands.

### Changed
- Documentation (`README.md` / `README.fa.md`) updated to cover transparent all-ports range mode, configurable KCP FEC and windows, the web dashboard, per-tunnel management commands, `.deb`/`.rpm` packages, self-healing upstream reconnect, and UDP-protocol support; version badge bumped to 0.15.0 and the status/roadmap refreshed.

## [0.15.0] - 2026-06-19

### Added
- Distribution packages: tagged releases now also publish `.deb` and `.rpm` packages for amd64 and arm64 (built with nfpm), alongside the raw binaries and SHA-256 checksums. The packages install `paqetpremium` to `/usr/local/bin`, declare a libpcap dependency, and bundle the example configs and docs under `/usr/share/doc/paqetpremium`. Install with `sudo dpkg -i paqetpremium_*.deb` (Debian/Ubuntu) or `sudo rpm -i paqetpremium-*.rpm` (RHEL/Fedora), then run the installer for guided setup or `paqetpremium run -c <config>`.

## [0.14.0] - 2026-06-19

### Added
- Self-healing upstreams: when an upstream's tunnel sessions die (server restart, network blip, or keepalive timeout) the client now rebuilds that upstream's connection pool out-of-band with exponential backoff and marks it healthy again once a ping succeeds - no manual client restart needed. Previously a dead pool stayed down until the client process was restarted.

### Changed
- Connection pool now skips dead smux sessions when opening a stream (round-robins over live sessions only) and exposes an Alive() check, so a single dead session no longer fails new connections while other sessions are still up.

## [0.13.0] - 2026-06-19

### Added
- Configurable KCP forward error correction and windows: `transport.kcp.data_shard`, `transport.kcp.parity_shard`, `transport.kcp.snd_wnd`, and `transport.kcp.rcv_wnd`. FEC trades a little bandwidth for far fewer retransmits on lossy links (common on Iran<->abroad paths) - e.g. `data_shard: 10` / `parity_shard: 3` recovers up to 3 lost packets per group without a round-trip. Defaults are unchanged (FEC off, role-based windows), so existing tunnels behave identically. Both ends must use the SAME data_shard/parity_shard. Validated by a new FEC integration test.

## [0.12.0] - 2026-06-19

### Added
- Web dashboard: the admin server now serves a self-contained dark-theme status page at its root (`/`) showing live download/upload throughput (with per-second rate), sessions, TCP/UDP and relay counters, error count, and a per-upstream health/RTT/sessions table, auto-refreshing every 2s. It reads the admin token from `?token=` when one is configured.
- Integration test harness: a full client<->server tunnel is exercised over loopback UDP PacketConns (no pcap engine or root needed), validating TCP echo over both KCP and QUIC and UDP datagram boundary preservation. Runs in CI on every push and locally on any platform.

### Changed
- `transport.Dial`/`transport.Listen` now accept a `net.PacketConn` (the pcap engine's Conn already satisfies it) instead of the concrete pcap type. Internal refactor with no runtime behavior change; it enables transport-level testing without pcap.

## [0.11.0] - 2026-06-19

### Fixed
- UDP relay now preserves datagram boundaries. Previously UDP payloads were copied as a raw byte stream over smux, which merged or split datagrams and broke UDP-based protocols (QUIC, Hysteria2, TUIC, WireGuard) and DNS under load. Both the client forward path and the server relay now use a length-prefixed datagram codec. Note: client and server must both run >= 0.11.0 for UDP forwarding; TCP is unaffected.

## [0.10.0] - 2026-06-19

### Added
- Transparent "tunnel all ports" range mode (client). A new `range:` config block installs an iptables nat REDIRECT for a port range (default `1-65535`, excluding SSH/22 and the redirect port) to a single local listener; each connection's original destination port is recovered via `SO_ORIGINAL_DST` and tunneled to `target_host:<port>` on the server. Reach any port on the server's localhost via the entry IP without per-port configuration. The client and multi-instance installer wizards offer it as "Tunnel ALL inbound ports transparently". The server needs no change (its relay already dials the per-connection target).

## [0.9.3] - 2026-06-19

### Fixed
- smux keepalive timeouts were assigned bare integers (`KeepAliveInterval = 2`, `KeepAliveTimeout = 8`). Because these fields are `time.Duration`, they were interpreted as 2 and 8 NANOSECONDS instead of seconds, so the keepalive watchdog fired immediately and tore down every smux session before any data could be exchanged. This produced `io: read/write on closed pipe` on the client and prevented the tunnel from ever establishing (the server logged "tunnel session accepted" but no stream ever completed). Corrected to a 10s keepalive interval and 30s timeout. This was the primary root cause of the "tunnel does not connect" failures.

## [0.9.2] - 2026-06-19

### Fixed
- pcap receive loop: tolerate `io.EOF` as a benign "no packet available" idle poll. Some libpcap builds in non-blocking/immediate mode return `io.EOF` (instead of a timeout) when no packet is ready; the loop previously treated this as fatal, which killed the kcp-go read loop on the very first idle poll, tore down the smux session (`io: read/write on closed pipe`), and stopped all transmission — so the server received zero packets and tunnels never established. This was the root cause of the "tunnel does not connect" failures on affected hosts.

### Added
- Installer: redesigned interactive UI (richer 256-color theme, sectioned layout, confirmation summary cards, readiness cards) and a multi-upstream client wizard. You can now add several exit servers, each with its own public IP, port, key and priority/weight, and pick the load-balancing strategy (failover / round_robin / weighted / least_latency). The server address is entered as separate IP and port prompts. The same multi-upstream flow is available in the multi-instance "add-tunnel" wizard.

## [0.9.1] - 2026-06-19

### Added
- pcap engine diagnostics: the transmit path now logs the first successful packet ("pcap transmit ok") and the first transmit failure ("pcap transmit failed") with the underlying error, and the receive path logs the first fatal read error. kcp-go swallows transmit errors, so this surfaces otherwise-invisible send failures for field debugging.

### Fixed
- pcap receive BPF filter no longer hard-codes an `ip6` clause when IPv6 is not configured; the `ip6` term is added only when an IPv6 address is present (matches the reference engine and avoids relying on libpcap IPv6 support on IPv4-only hosts).
- Installer (`install-premium.sh`): removed an over-eager `set -e` + `ERR` trap (and a `BASH_COMMAND` typo) that aborted the interactive wizard with a cryptic "unbound variable" message right after a successful install, triggered by benign non-zero exits from detection commands (`awk ... exit` raising SIGPIPE under `pipefail`). Critical steps (Go/toolchain download, git clone, build, binary install) now fail explicitly and service starts are non-fatal with log hints.
- Installer: NAT awareness. The server wizard detects the host's public IPv4 and tells the operator which address to configure on clients, warning when the interface IP differs from the public IP (server behind NAT). The client and multi-instance wizards prompt for the server's PUBLIC address and warn on obviously-private inputs. This prevents pointing the client at the server's internal/NAT IP, which silently dropped all tunnel traffic.

## [0.9.0] - 2026-06-19

This release makes the project build and run correctly on its only supported
platform (Linux), hardens the QUIC transport, and ships a professional
installer/manager.

### Fixed
- **Linux build failure (critical).** `internal/pcap/send_linux.go` named a function parameter `net`, shadowing the imported `net` package, so `net.IP(...)`/`net.HardwareAddr(...)` failed to compile on Linux. The parameter was renamed and the package now type-checks.
- **Per-peer address keying.** `netutil.AddrKey` mis-packed IPv4 octets (the 4th octet collided with the 2nd) and returned `0` for every IPv6 address, which could apply the wrong TCP-flag cycle to the wrong peer. It now uses an FNV-1a hash over the canonical 16-byte IP plus port, producing distinct, stable keys for IPv4 and IPv6.
- **IPv6 addresses dropped.** `netutil.ParseUDPAddr` stored `ip.To4()`, which is `nil` for IPv6, silently discarding IPv6 server/upstream/listen addresses. IPv6 is now preserved while IPv4 normalization is retained.
- **Misleading connectivity check.** `tunnel.PingConfig` always returned `nil`, so `paqetpremium test` reported "server reachable" even when it was not. It now performs a real ping round-trip (via the new `Manager.PingAll`) and surfaces failures.

### Security
- **QUIC peer authentication.** Previously the QUIC client used `InsecureSkipVerify` and the server requested no client certificate, so the shared secret provided no authentication on the QUIC path. Both sides now present and **pin** a certificate derived deterministically from the shared secret (mutual authentication); a peer configured with a different secret is rejected. KCP keying is unchanged.
- Certificate derivation was made fully deterministic (fixed validity timestamps and a deterministic key/signature path) so both endpoints compute byte-identical certificates for pinning.

### Added
- **Professional installer & manager** (`install-premium.sh`) wired to this repository: guided server/client wizards, KCP/QUIC selection, multi-upstream, port-forward, SOCKS5, optional IPv6, admin token; cross-distro dependency install; automatic Go toolchain provisioning; build-from-source; systemd single and multi-instance units with `SIGHUP` reload; and `status`/`list`/`logs`/`reload`/`restart`/`update`/`uninstall` commands with a post-start health check.
- **One-line bootstrap** (`scripts/install-linux.sh`) that fetches and runs the installer.
- Bilingual documentation: English (`README.md`) and Persian (`README.fa.md`).

### Changed
- Version bumped to `0.9.0`.

## [0.8.0-dev]
- Dual transport: KCP or QUIC, selectable via `transport.protocol`.
- SOCKS5 UDP ASSOCIATE; multi-upstream strategies and health checks; admin API, metrics, IPv6; reload/bench CLI; arm64 target.

[0.9.0]: https://github.com/iPmartNetwork/paqetpremium/releases/tag/v0.9.0
[0.8.0-dev]: https://github.com/iPmartNetwork/paqetpremium
