# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Installer (`install-premium.sh`): removed an over-eager `set -e` + `ERR` trap (and a `BASH_COMMAND` typo) that aborted the interactive wizard with a cryptic "unbound variable" message right after a successful install, triggered by benign non-zero exits from detection commands (`awk ... exit` raising SIGPIPE under `pipefail`). Critical steps (Go/toolchain download, git clone, build, binary install) now fail explicitly and service starts are non-fatal with log hints.
- Installer: NAT awareness. The server wizard now detects the host's public IPv4 and explicitly tells the operator which address to configure on the client, warning when the interface IP differs from the public IP (i.e. the server is behind NAT). The client wizard now prompts for the server's PUBLIC address and warns on obviously-private inputs. This prevents pointing the client at the server's internal/NAT IP, which silently dropped all tunnel traffic.

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
