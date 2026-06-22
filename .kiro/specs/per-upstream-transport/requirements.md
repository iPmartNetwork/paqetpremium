# Requirements Document

## Introduction

PaqetPremium clients can already connect to multiple upstream (Kharej / exit) servers with load-balancing strategies (failover, round_robin, weighted, least_latency). However, the transport configuration (protocol kcp/quic, MTU, KCP windows/FEC, QUIC settings) is currently global: every upstream must use the same transport. Operators want to mix transports per upstream (for example one exit over KCP and another over QUIC) to adapt to different network paths and DPI conditions between Iran and abroad.

This spec adds per-upstream transport selection while preserving full backward compatibility with existing single-transport configs. It also bundles closely related multi-upstream reliability fixes uncovered during field testing: a false-negative in the `test` command when range mode is used, kernel RST suppression on the fake-TCP tunnel port for stateful/DPI paths, and clearer strategy guidance for range mode.

The client dial path is already per-upstream: each upstream is dialed via `tunnelpool.Dial` with its own `TransportConfig`, and `config.TransportForKey` currently only injects the per-server secret key. The change is therefore concentrated in the config data model, the transport resolution/merge logic, validation, the `test` command output, and the installer wizard. No server-side protocol changes are required, because each exit server already runs independently with its own transport; the Iran client simply needs to match each upstream's protocol.

Out of scope: cutting a new release that ships the dashboard config editor in prebuilt binaries (tracked separately), and the TLS/HTTP2 obfuscation TCP-fallback transport (a future, larger effort).

## Requirements

### Requirement 1: Per-upstream transport override

**User Story:** As an operator, I want each upstream server to optionally specify its own transport protocol and tuning, so that I can run different exit servers over different transports (KCP or QUIC) from a single Iran client.

#### Acceptance Criteria
1. WHEN an `upstream.servers[]` entry includes a `transport` block THEN the client SHALL use that transport (protocol and tuning) for all connections to that upstream.
2. WHEN an `upstream.servers[]` entry omits the `transport` block THEN the client SHALL use the global `transport` configuration for that upstream.
3. WHEN a per-upstream `transport` block sets some fields but omits others THEN the client SHALL merge the provided fields over the global transport, using global values for omitted fields.
4. WHEN the per-upstream transport omits a secret key THEN the client SHALL resolve the key from the per-server `key` (if set), otherwise the global key, preserving current key-resolution behavior.
5. The per-upstream transport SHALL support the same options as the global transport (kcp: mode, block, mtu, data_shard, parity_shard, snd_wnd, rcv_wnd; quic: alpn, idle timeouts).

### Requirement 2: Backward compatibility

**User Story:** As an existing user, I want my current configs (single global transport, no per-upstream transport) to keep working unchanged, so that upgrading does not break my tunnels.

#### Acceptance Criteria
1. WHEN a config has no per-upstream `transport` blocks THEN the client SHALL behave identically to the previous version.
2. WHEN a config uses the legacy single `server.addr` form (no upstream list) THEN the client SHALL continue to use the global transport.
3. The global `transport` block SHALL remain required and continue to serve as the default for all upstreams.

### Requirement 3: Per-upstream transport validation

**User Story:** As an operator, I want invalid per-upstream transport settings to be rejected with a clear error, so that I catch misconfigurations before runtime.

#### Acceptance Criteria
1. WHEN a per-upstream transport specifies a protocol other than kcp or quic THEN validation SHALL fail with an error naming the offending upstream.
2. WHEN a per-upstream transport has invalid KCP parameters (negative shards, parity without data, shard sum greater than 255, negative windows) THEN validation SHALL fail with an error naming the offending upstream.
3. WHEN a per-upstream transport omits optional fields THEN validation SHALL apply the same defaults as the global transport (mtu 1150, mode fast, block aes-128-gcm, quic alpn paqetpremium).
4. WHEN `paqetpremium test` runs THEN it SHALL report the resolved transport protocol for each upstream.

### Requirement 4: Installer wizard per-upstream transport

**User Story:** As an operator using the installer, I want to choose the transport for each upstream during the client wizard, so that I can set up mixed transports without hand-editing YAML.

#### Acceptance Criteria
1. WHEN adding upstream servers in the client wizard THEN the installer SHALL offer a transport choice (kcp/quic) per upstream.
2. WHEN the operator selects the same transport for every upstream THEN the installer MAY emit a single global transport with no per-upstream overrides, to keep configs clean.
3. WHEN the operator selects different transports across upstreams THEN the installer SHALL emit per-upstream `transport` blocks accordingly.
4. The emitted config SHALL pass `paqetpremium test` validation.

### Requirement 5: Fix range-mode false negative in the test command

**User Story:** As an operator using range mode, I want `paqetpremium test` to recognize range mode as a valid forwarding configuration, so that the readiness check does not report a false failure.

#### Acceptance Criteria
1. WHEN range mode is enabled and no forward/socks5 rules are set THEN the `test` command SHALL NOT report "forward/socks5 rules configured" as a failure.
2. WHEN range mode is enabled THEN the `test` command SHALL report the range configuration (ports, target host, redirect port) as a passing check.
3. WHEN neither forward, socks5, nor range is configured on a client THEN the `test` command SHALL still report a failure (current behavior preserved).

### Requirement 6: Kernel RST suppression on tunnel ports

**User Story:** As an operator on a stateful or DPI-filtered path, I want the kernel's TCP RST on the fake-TCP tunnel port suppressed, so that middleboxes do not tear down the flow and data passes reliably.

#### Acceptance Criteria
1. WHEN the server is installed or configured THEN the installer SHALL add an iptables rule that prevents the kernel from emitting RST for inbound packets on the tunnel listen port, while leaving pcap capture intact.
2. WHEN the client is installed or configured THEN the installer SHALL add an iptables rule that prevents the kernel from emitting RST in response to inbound packets from each upstream's port.
3. WHEN a tunnel is uninstalled THEN the installer SHALL remove the iptables rules it added.
4. WHEN the installer is re-run (idempotent) THEN it SHALL NOT create duplicate rules.
5. The RST-suppression behavior SHALL be documented in EN and FA, including the manual iptables commands for users who installed from prebuilt binaries.

### Requirement 7: Strategy guidance for range mode

**User Story:** As an operator using range mode with multiple upstreams, I want guidance on strategy selection, so that I do not break connections by spreading them across servers that lack the target service.

#### Acceptance Criteria
1. WHEN range mode is enabled with more than one upstream and a non-failover strategy is selected THEN the installer wizard SHALL warn that every exit server must expose the same target service on the configured target host.
2. The documentation SHALL explain when to use failover versus load-balancing strategies with range mode.
