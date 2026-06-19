# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
