# Requirements Document

## Introduction

PaqetPremium's multi-upstream support currently treats exit servers as interchangeable via load-balancing strategies (failover, round_robin, weighted, least_latency). This fits mirrored exits, but not the common real-world topology where each exit server hosts DIFFERENT inbounds (for example a panel on one server and a node on another, each with its own ports). In that case "send all traffic to one active exit" (failover) or transparent all-ports range mode cannot deliver a connection to the correct exit.

This spec adds an explicit, port-based multi-server forwarding mode driven by the installer wizard. For each exit (upstream), the operator enters the TCP ports and optionally the UDP ports that belong to that exit. The Iran client then listens on each of those ports and forwards every connection, port-preserving, to the bound exit server over the tunnel (KCP or QUIC). Routing is by destination port, which is the only signal the entry server has for transparent forwarding; this works because panel inbounds use distinct ports per server.

Key properties: it is genuinely multi-server (each port routes to its own exit, not a single active one), transparent and port-preserving (end users change only the server address to the Iran IP, never the port), supports both TCP and UDP without TPROXY (because specific ports are listened on, not the whole range), and requires NO changes on the exit (Kharej) servers — the server relay already dials the target the client sends.

The underlying binary already supports per-rule routing: `ForwardRule` has a `bind_upstream` field and the manager can open streams on a named upstream pool, and UDP forwarding (length-prefixed datagram framing) exists. The work is therefore concentrated in the installer wizard, config validation of `bind_upstream`, verification that the forward data path honors `bind_upstream` for both TCP and UDP, and documentation.

Out of scope: transparent all-ports UDP via TPROXY; changes to the exit-server relay.

## Requirements

### Requirement 1: Per-server port collection in the client wizard

**User Story:** As an operator, I want the installer to ask for each exit server's TCP and UDP ports separately, so that each port is tied to the correct exit.

#### Acceptance Criteria
1. WHEN adding upstream servers in the client wizard THEN for EACH upstream the installer SHALL prompt for a comma-separated list of TCP ports (e.g. `443,2050,33658`).
2. WHEN the TCP ports are entered THEN the installer SHALL ask whether the server also has UDP ports (y/n).
3. WHEN the operator answers yes THEN the installer SHALL prompt for a comma-separated list of UDP ports; WHEN the operator answers no THEN the installer SHALL continue without UDP ports.
4. WHEN a port list contains invalid entries (non-numeric or out of 1-65535) THEN the installer SHALL reject the entry and re-prompt.
5. WHEN no ports are entered for a server THEN the installer SHALL warn that the server will receive no traffic and allow the operator to continue or re-enter.

### Requirement 2: Config generation with port-preserving, upstream-bound forward rules

**User Story:** As an operator, I want the wizard to emit forward rules that route each port to its exit, so that the running config matches my intent without hand-editing.

#### Acceptance Criteria
1. WHEN the config is written THEN for each TCP port of an upstream the installer SHALL emit a forward rule with `listen: "0.0.0.0:<port>"`, `target: "127.0.0.1:<port>"`, `protocol: tcp`, and `bind_upstream: <upstream name>`.
2. WHEN UDP ports were provided for an upstream THEN the installer SHALL emit equivalent forward rules with `protocol: udp` and the same `bind_upstream`.
3. The emitted forward rules SHALL preserve the port (listen port equals target port).
4. WHEN all port lists are collected THEN the generated config SHALL pass `paqetpremium test`.

### Requirement 3: Wizard mode selection

**User Story:** As an operator, I want this multi-server port forwarding to be the guided path for multi-exit setups, while keeping existing modes available.

#### Acceptance Criteria
1. WHEN running the client wizard THEN the installer SHALL offer this explicit per-server port forwarding flow in place of the previous transparent all-ports range prompt as the default for multi-exit setups.
2. The range engine in the binary SHALL remain available for single-exit/manual use (no removal of the `range` config support).
3. WHEN only forward ports are configured (no range, no socks5) THEN `paqetpremium test` SHALL treat the client as validly configured.

### Requirement 4: bind_upstream validation

**User Story:** As an operator, I want a clear error if a forward rule points to a non-existent upstream, so that typos are caught at load time.

#### Acceptance Criteria
1. WHEN a forward rule sets `bind_upstream` to a name that does not match any `upstream.servers[].name` THEN `Config.Validate` SHALL fail with an error naming the offending rule and the unknown upstream.
2. WHEN `bind_upstream` is empty THEN the forward rule SHALL use the default upstream selection (current behavior), and validation SHALL pass.
3. WHEN `bind_upstream` matches an existing upstream name THEN validation SHALL pass.

### Requirement 5: UDP forwarding honors bind_upstream

**User Story:** As an operator, I want UDP forward rules to reach the same bound exit as TCP, so that UDP-based inbounds work in the multi-server setup.

#### Acceptance Criteria
1. WHEN a UDP forward rule has a `bind_upstream` THEN the client SHALL open the UDP relay stream on that upstream's pool (not the default selection).
2. WHEN a UDP forward rule has no `bind_upstream` THEN it SHALL use the default upstream selection.
3. The UDP data path SHALL preserve datagram boundaries end-to-end (existing framing).

### Requirement 6: No exit-server changes

**User Story:** As an operator, I want to add or change client-side port mappings without touching the exit servers, so that scaling to many users and ports is low-friction.

#### Acceptance Criteria
1. WHEN the client forward mappings change THEN the exit (Kharej) server config SHALL NOT require any change; the relay dials the target the client sends.
2. The documentation SHALL state that only the panel/inbound must listen on the target host (127.0.0.1 or 0.0.0.0) on the exit server, and that end-user client configs change only the server address (to the Iran IP), never the port.

### Requirement 7: Backward compatibility

**User Story:** As an existing user, I want my current forward/range/socks5 configs to keep working, so that upgrading is safe.

#### Acceptance Criteria
1. WHEN a config has forward rules without `bind_upstream` THEN behavior SHALL be unchanged.
2. WHEN a config uses range or socks5 THEN behavior SHALL be unchanged.
3. The new validation SHALL only reject `bind_upstream` values that reference unknown upstreams.

### Requirement 8: Documentation

**User Story:** As a user, I want the multi-server port forwarding documented in both languages, so that I can set it up correctly.

#### Acceptance Criteria
1. The README (EN and FA) SHALL document the per-server TCP/UDP port forwarding flow with a config example and the "address-only change" note for end users.
2. The CHANGELOG SHALL record the new mode under `[Unreleased]`.