# Implementation Plan

- [x] 1. Add bind_upstream validation in config
  - In `internal/config/config.go`, add `func (c *Config) upstreamNameSet() map[string]bool` that returns valid upstream names exactly as `UpstreamEndpoints()` derives them (each `upstream.servers[i].name`, falling back to `upstream-<i+1>` when empty; plus `default` when the legacy single `server.addr` form is used).
  - In `Validate()` (client role), for each `forward` rule with non-empty `BindUpstream` not in the set, return an error naming the rule's `Listen` and the bad value (e.g. `forward rule "0.0.0.0:443": bind_upstream "XX" does not match any upstream`). Apply the same check to `range.bind_upstream` when set. Empty `bind_upstream` stays valid.
  - _Requirements: 4.1, 4.2, 4.3, 7.1, 7.3_

- [x] 2. Unit tests for bind_upstream validation
  - Add tests in `internal/config`: forward rule with unknown `bind_upstream` fails with a naming error; matching name passes; empty `bind_upstream` passes; legacy `server.addr` form accepts `bind_upstream: default`; `upstreamNameSet` returns the expected names including the `upstream-<i+1>` fallback.
  - _Requirements: 4.1, 4.2, 4.3, 7.1_

- [x] 3. Installer: per-server TCP/UDP port collection
  - In `install-premium.sh`, add global arrays `UP_TCP_PORTS=()` and `UP_UDP_PORTS=()` and a `validate_ports <csv>` helper (each comma item an int in 1..65535; blanks ignored; returns non-zero on invalid).
  - In `add_upstream_servers`, after the per-upstream transport prompt: prompt a CSV of TCP ports (re-prompt until valid; empty allowed); `ask_yes_no` whether the server has UDP ports and if yes prompt a validated CSV of UDP ports; append both (CSV string or empty) to the new arrays. If both are empty for a server, `warn` it will receive no traffic.
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_

- [x] 4. Installer: emit bound forward rules; drop range/global-forward prompts
  - In `write_client_config`, replace the `range:` and global `forward:` emission with generated forward rules: for each upstream index, emit one rule per TCP port (`listen 0.0.0.0:<p>`, `target 127.0.0.1:<p>`, `protocol tcp`, `bind_upstream <UP_NAMES[i]>`) and one per UDP port (`protocol udp`). Preserve the port. Keep emitting `socks5:` only when a socks port was chosen; do not emit a `range:` block. Keep the per-upstream `transport:` override block under each server unchanged.
  - In `wizard_client` and `wizard_add_tunnel`, remove the `ask_range_mode` call and the global forward-ports prompt; keep admin/token and an optional socks5 prompt.
  - Verify a generated multi-server config (DE+FN, TCP+UDP, distinct ports) passes `paqetpremium test`. Run `bash -n install-premium.sh`.
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 3.1, 3.2, 3.3, 6.1_

- [x] 5. Documentation and changelog
  - Update `README.md` and `README.fa.md`: document the per-server TCP/UDP port forwarding flow with a mixed DE/FN example, the distinct-ports constraint, the "end users change only the address (Iran IP), never the port" note, and that no exit-server change is needed.
  - Add a CHANGELOG `[Unreleased]` entry for the new multi-server port forwarding mode and the `bind_upstream` validation.
  - _Requirements: 6.2, 8.1, 8.2_

- [x] 6. Final verification
  - Run `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l internal/`, and `bash -n install-premium.sh`; fix any issues.
  - _Requirements: 2.4, 3.3, 4.1_
