# Design Document

## Overview

This feature makes the Iran client front MULTIPLE exit servers with different inbounds by routing each published port to its own exit. The data path already supports this: `forward.Manager` resolves a per-rule upstream via `m.opener(rule.BindUpstream)` -> `RouteFn(bind)` -> `upstream.Manager.ForServer(name)`, for BOTH TCP and UDP. So no transport/relay code changes are needed. The work is: (1) collect per-server TCP/UDP ports in the installer wizard and emit `forward` rules bound to each upstream, (2) validate `bind_upstream` references at config load, (3) documentation. No exit-server changes are required because the server relay dials whatever `target` the client sends.

## Architecture

```
client wizard (per upstream server):
  TCP ports: 443,2050,33658
  UDP ports? y -> 443,8443   (or n)

write_client_config -> forward rules (port-preserving, bound):
  - listen 0.0.0.0:443  target 127.0.0.1:443  protocol tcp  bind_upstream DE
  - listen 0.0.0.0:2050 target 127.0.0.1:2050 protocol tcp  bind_upstream DE
  - listen 0.0.0.0:443  target 127.0.0.1:443  protocol udp  bind_upstream DE
  - listen 0.0.0.0:8443 target 127.0.0.1:8443 protocol tcp  bind_upstream FN

runtime data path (already implemented):
  inbound :PORT -> forward.Manager -> opener(bind) -> Manager.ForServer(DE|FN)
                -> that upstream's pool -> tunnel -> exit 127.0.0.1:PORT
```

## Components and Interfaces

### 1. bind_upstream validation (`internal/config/config.go`)

Add validation so a typo fails fast. In `Validate()`:
- Build the set of valid upstream names exactly as `UpstreamEndpoints()` derives them: each `upstream.servers[i].name` (falling back to `upstream-<i+1>` when empty); plus `default` when the legacy single `server.addr` form is used.
- For each `forward` rule with a non-empty `BindUpstream`, error if it is not in the set: `forward rule %q: bind_upstream %q does not match any upstream` (use the rule's Listen for %q).
- Apply the same check to `range.bind_upstream` if set.
- Empty `bind_upstream` is valid (default selection).

Helper: `func (c *Config) upstreamNameSet() map[string]bool`.

### 2. Installer wizard (`install-premium.sh`)

Replace the transparent all-ports range prompt with explicit per-server port collection.

Global state: add arrays `UP_TCP_PORTS=()` and `UP_UDP_PORTS=()` next to the existing `UP_*` arrays.

Helper `validate_ports <csv>`: returns 0 if every comma item is an integer in 1..65535 (ignoring blanks); else non-zero. Reused for TCP and UDP prompts (re-prompt on invalid).

In `add_upstream_servers`, after the per-upstream transport prompt, for the current server:
- `prompt` a CSV of TCP ports; loop until `validate_ports` passes (empty allowed but warn per Req 1.5).
- `ask_yes_no` "Does this server have UDP ports?"; if yes, prompt a CSV of UDP ports with the same validation.
- Append to `UP_TCP_PORTS` / `UP_UDP_PORTS` (store the CSV string per index; use empty string when none).
- If both TCP and UDP are empty for a server, `warn` that it will receive no traffic.

Remove the `ask_range_mode` call and the global forward-ports prompt from `wizard_client` and `wizard_add_tunnel`. Keep the admin/token prompt; keep an OPTIONAL socks5 prompt. (The `range`/socks paths in the binary are unchanged; the wizard simply stops offering range.)

In `write_client_config`, replace the `range:`/global-`forward:` emission with generated forward rules:
```
echo "forward:"
for i in indices:
  for p in split(UP_TCP_PORTS[i], ","):  # skip blanks
    - listen "0.0.0.0:p"; target "127.0.0.1:p"; protocol tcp; bind_upstream UP_NAMES[i]
  for p in split(UP_UDP_PORTS[i], ","):
    - listen "0.0.0.0:p"; target "127.0.0.1:p"; protocol udp; bind_upstream UP_NAMES[i]
```
Keep emitting `socks5:` only if a socks port was chosen. Do not emit a `range:` block.

Note: the per-upstream `transport:` override block (from the per-upstream-transport feature) must still be emitted under each server in the `upstream.servers` list; that logic is unchanged.

### 3. Forward data path (no change)

`forward.Manager.serveTCP`/`serveUDP` already call `m.opener(rule.BindUpstream)` and use `OpenTCP`/`OpenUDP` on the bound opener; `RouteFn` is wired to `upstream.Manager.ForServer` in the client. Confirm this wiring exists; make no functional changes. UDP datagram boundaries are preserved by the existing length-prefixed codec.

### 4. test command (`internal/app/test.go`) (no change)

The forward/socks5/range readiness check already passes when forward rules exist (fixed in the previous spec). Per-upstream protocol reporting already lists each upstream. No change required.

### 5. Documentation (`README.md`, `README.fa.md`, `CHANGELOG.md`)

Document the per-server TCP/UDP port forwarding flow with a mixed example, the rule that each exit's ports must be distinct, the "end users change only the address (Iran IP), never the port" note, and that no exit-server change is needed. Add a CHANGELOG `[Unreleased]` entry.

## Data Models

Example client config (two exits, different inbounds, TCP + UDP):

```yaml
role: client
upstream:
  strategy: failover
  servers:
    - name: DE
      addr: 116.203.19.246:22490
      key: SECRET
      priority: 1
    - name: FN
      addr: 157.180.65.244:22491
      key: SECRET
      priority: 2
forward:
  - { listen: "0.0.0.0:443",  target: "127.0.0.1:443",  protocol: tcp, bind_upstream: DE }
  - { listen: "0.0.0.0:2050", target: "127.0.0.1:2050", protocol: tcp, bind_upstream: DE }
  - { listen: "0.0.0.0:443",  target: "127.0.0.1:443",  protocol: udp, bind_upstream: DE }
  - { listen: "0.0.0.0:8443", target: "127.0.0.1:8443", protocol: tcp, bind_upstream: FN }
transport:
  protocol: kcp
  kcp: { key: SECRET, mtu: 1150 }
```

## Error Handling

- Unknown `bind_upstream` -> `Config.Validate` fails fast naming the rule and the bad value.
- Invalid port entry in the wizard -> re-prompt; never writes an invalid config.
- A server with no ports -> warning, but allowed (operator may add ports later via edit).
- Two exits sharing the same port number cannot be disambiguated (routing is by port); documented as a constraint.

## Testing Strategy

Unit tests (pure Go, no root/pcap):
- `Config.Validate`: forward rule with unknown `bind_upstream` fails with a naming error; matching name passes; empty `bind_upstream` passes; legacy `server.addr` accepts `default`.
- `upstreamNameSet`: returns expected names including the `upstream-<i+1>` fallback and `default`.
- `TestConfig`: a client with only bound forward rules (no range/socks) passes and reports each upstream.

Manual/integration:
- Installer-generated multi-server config (DE+FN, TCP+UDP) passes `paqetpremium test`.
- A TCP connection to the Iran IP on a DE port reaches DE's localhost service; an FN port reaches FN; verified via dashboard relay counters per upstream.
