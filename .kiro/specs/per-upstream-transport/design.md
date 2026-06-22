# Design Document

## Overview

This design adds per-upstream transport selection to the PaqetPremium client and bundles related multi-upstream reliability fixes. The client dial path is already per-upstream (`upstream.Manager` calls `tunnelpool.Dial(..., transportCfg, ...)` once per upstream), so the work concentrates in the config model, transport resolution/merge, validation, the `test` command, the installer wizard, and kernel RST suppression. No server-side protocol changes are required.

Design principle: zero-value inheritance. A per-upstream `transport` block is a partial override. Any field left at its zero value is inherited from the global `transport`. This keeps per-upstream blocks small and makes omission unambiguous.

## Architecture

```
client.yaml
  transport:            (global defaults, still required)
  upstream.servers[]:
    - transport: {...}   (optional partial override)

Config.UpstreamEndpoints()
  for each server:
    ep.Transport = mergeTransport(global, server.transport)
    ep.Transport.KCP.Key = resolvedKey(server)   // per-server key or global key
    -> []UpstreamEndpoint{ ..., Transport }

upstream.Manager.NewManager / Reload / reconnect
    tunnelpool.Dial(ctx, cfg, ep.Addr, ep.Transport, remoteFlags)   // was cfg.TransportForKey(ep.Key)
```

Validation merges the same way at load time so per-upstream errors are caught before runtime.

## Components and Interfaces

### 1. Config data model (`internal/config/config.go`)

Add an optional transport override to `UpstreamServer`:

```go
type UpstreamServer struct {
    Name      string           `yaml:"name"`
    Addr      string           `yaml:"addr"`
    Key       string           `yaml:"key"`
    Weight    int              `yaml:"weight"`
    Priority  int              `yaml:"priority"`
    Transport *TransportConfig `yaml:"transport,omitempty"` // NEW: partial override
}
```

`TransportConfig`, `KCPConfig`, `QUICConfig` are unchanged and reused for the override.

### 2. Resolved endpoint (`internal/config/upstream.go`)

Add the fully merged transport to `UpstreamEndpoint`:

```go
type UpstreamEndpoint struct {
    Name      string
    Addr      *net.UDPAddr
    Key       string
    Weight    int
    Priority  int
    Transport TransportConfig // NEW: merged global+override, key injected
}
```

### 3. Transport merge (`internal/config/upstream.go`)

```go
// mergeTransport overlays the non-zero fields of override onto base.
func mergeTransport(base TransportConfig, override *TransportConfig) TransportConfig {
    t := base
    if override == nil {
        return t
    }
    if override.Protocol != "" { t.Protocol = override.Protocol }
    if override.Conn > 0       { t.Conn = override.Conn }
    if override.KCP.Mode != ""  { t.KCP.Mode = override.KCP.Mode }
    if override.KCP.Block != "" { t.KCP.Block = override.KCP.Block }
    if override.KCP.Key != ""   { t.KCP.Key = override.KCP.Key }
    if override.KCP.MTU > 0      { t.KCP.MTU = override.KCP.MTU }
    if override.KCP.DataShard > 0   { t.KCP.DataShard = override.KCP.DataShard }
    if override.KCP.ParityShard > 0 { t.KCP.ParityShard = override.KCP.ParityShard }
    if override.KCP.SndWnd > 0  { t.KCP.SndWnd = override.KCP.SndWnd }
    if override.KCP.RcvWnd > 0  { t.KCP.RcvWnd = override.KCP.RcvWnd }
    if override.QUIC.ALPN != ""            { t.QUIC.ALPN = override.QUIC.ALPN }
    if override.QUIC.IdleTimeout != ""     { t.QUIC.IdleTimeout = override.QUIC.IdleTimeout }
    if override.QUIC.MaxIdleTimeout != ""  { t.QUIC.MaxIdleTimeout = override.QUIC.MaxIdleTimeout }
    return t
}
```

Note: FEC shards and windows treat 0 as "inherit". Since FEC defaults off, this matches operator expectations; documented in README.

### 4. Endpoint resolution

In `UpstreamEndpoints()`, after computing the resolved per-server key, set:

```go
tr := mergeTransport(c.Transport, s.Transport)
tr.KCP.Key = key // resolved per-server or global key
out = append(out, UpstreamEndpoint{..., Transport: tr})
```

The legacy single `server.addr` path sets `Transport: c.Transport` (global, unchanged).

`TransportForKey` is retained for compatibility but the manager switches to `ep.Transport`.

### 5. Manager wiring (`internal/upstream/manager.go`)

Replace `cfg.TransportForKey(ep.Key)` with `ep.Transport` in three places: `NewManager`, `Reload`, and `reconnect` (which uses `cur.ep.Transport`). No other manager logic changes.

### 6. Validation (`internal/config/config.go`)

Refactor the inline global-transport validation into a reusable method that fills defaults and validates, parameterized by a label for error messages:

```go
func (c *Config) validateTransport(t *TransportConfig, label string) error
```

- Call it for the global transport (label "transport").
- For each upstream server with `Transport != nil`, validate the MERGED transport (so inherited defaults apply) with label `upstream.servers[i] (name)`.
- Errors name the offending upstream, e.g. `upstream "kharej-2": transport.protocol: must be kcp or quic`.

The existing per-server `key is required` check is unchanged; the merged transport's key is the resolved key, so kcp key presence is guaranteed.

### 7. `test` command (`internal/app/test.go`)

Requirement 5 (range false negative): replace

```go
add(len(cfg.Forward) > 0 || len(cfg.SOCKS5) > 0, "forward/socks5 rules configured")
```

with

```go
rangeOn := cfg.Range != nil && cfg.Range.Enabled
hasRules := len(cfg.Forward) > 0 || len(cfg.SOCKS5) > 0 || rangeOn
add(hasRules, "forward/socks5/range rules configured")
if rangeOn {
    add(true, fmt.Sprintf("range mode: ports=%s target=%s redirect=:%d",
        cfg.Range.Ports, cfg.Range.TargetHost, cfg.Range.RedirectPort))
}
```

Requirement 3.4 (per-upstream protocol): after the upstream-servers count check, when role is client and `cfg.Upstream != nil`, resolve endpoints and report each:

```go
if eps, err := cfg.UpstreamEndpoints(); err == nil {
    for _, ep := range eps {
        add(true, fmt.Sprintf("upstream %s -> %s (%s)", ep.Name, ep.Addr, ep.Transport.Protocol))
    }
}
```

### 8. Installer wizard (`install-premium.sh`)

`add_upstream_servers` currently calls `ask_transport` once. Change:
- Keep a global transport prompt for defaults (first selection becomes the global block).
- For each upstream, prompt its transport (default = global choice), storing into a new array `UP_TRANSPORTS[i]`.
- In `write_client_config`, after emitting each server's name/addr/key/priority/weight, if `UP_TRANSPORTS[i]` differs from the global protocol, emit a nested per-server `transport:` block:

```yaml
    - name: kharej-2
      addr: 1.2.3.4:20202
      key: <secret>
      priority: 2
      weight: 1
      transport:
        protocol: quic
```

- If all upstreams use the same transport, emit no per-server blocks (clean config; Requirement 4.2).
- Requirement 7.1: if range is enabled, more than one upstream exists, and strategy is not failover, print a warning that all exit servers must expose the same target service on the configured target host.

### 9. Kernel RST suppression (`install-premium.sh` + systemd units)

Goal: stop the kernel from emitting TCP RST on the fake-TCP tunnel port, while pcap still captures (pcap taps at AF_PACKET before netfilter). Surgical approach using an OUTPUT rule that drops only RST packets:

- Server unit (listen port P): `iptables -C/-A OUTPUT -p tcp --sport P --tcp-flags RST RST -j DROP -m comment --comment paqetpremium`
- Client unit (per upstream port P): `iptables -C/-A OUTPUT -p tcp --dport P --tcp-flags RST RST -j DROP -m comment --comment paqetpremium`

Lifecycle: tie rules to the service via systemd so they survive reboots and are cleaned up automatically:
- `ExecStartPre=` runs an idempotent add (`iptables -C ... || iptables -A ...`).
- `ExecStopPost=` runs the matching delete (`iptables -D ...`).
- When IPv6 is configured, mirror with `ip6tables`.

The installer generates these ExecStartPre/ExecStopPost lines into the unit file using the known port(s). `cmd_uninstall` additionally sweeps any leftover rules tagged with the `paqetpremium` comment. Re-running the installer is idempotent because of the `-C` guard.

### 10. Documentation (`README.md`, `README.fa.md`, `CHANGELOG.md`)

- Per-upstream transport: YAML example mixing kcp and quic upstreams; note zero-value inheritance.
- RST suppression: what it does and the manual iptables commands for prebuilt-binary users.
- Strategy guidance: failover vs load-balancing with range mode.
- CHANGELOG Unreleased entries.

## Data Models

Example client config with mixed transports:

```yaml
role: client
transport:            # global defaults
  protocol: kcp
  conn: 6
  kcp:
    key: SHARED_SECRET
    mtu: 1150
upstream:
  strategy: failover
  servers:
    - name: kharej-1   # inherits global kcp
      addr: 116.203.19.246:20201
      key: SHARED_SECRET
      priority: 1
    - name: kharej-2   # overrides to quic
      addr: 157.180.65.244:20202
      key: SHARED_SECRET
      priority: 2
      transport:
        protocol: quic
```

## Error Handling

- Invalid per-upstream protocol or KCP params: `Config.Validate` returns an error naming the upstream; load fails fast.
- Merge never panics on nil override (guarded).
- `test` reports per-upstream protocol and range status without failing for range-only clients.
- Installer iptables operations are guarded by `-C` checks; failures to add a rule warn but do not abort the install (consistent with existing non-fatal service-start handling).

## Testing Strategy

Unit tests (pure Go, run in CI and locally; no pcap/root needed):
- `mergeTransport`: nil override returns base; partial override overlays only set fields; full override replaces; key injection precedence (per-server key > global key).
- `UpstreamEndpoints`: per-server transport resolved and merged; legacy `server.addr` uses global; ordering by priority preserved.
- `Config.Validate`: rejects bad per-upstream protocol and KCP params with an error naming the upstream; valid mixed config passes; config with no overrides behaves identically (backward compatibility).
- `TestConfig` (test command): range-only client passes the forward/socks5/range check and emits a range-mode line; client with neither forward/socks5/range still fails.

Manual/integration:
- Installer-generated mixed-transport config passes `paqetpremium test`.
- RST rules added on service start, removed on stop, idempotent on re-run (verified with `iptables -S`).
