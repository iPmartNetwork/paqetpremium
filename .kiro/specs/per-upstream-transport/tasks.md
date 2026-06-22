# Implementation Plan

- [x] 1. Add per-upstream transport to the config data model and merge logic
  - Add optional field `Transport *TransportConfig` (yaml `transport,omitempty`) to `UpstreamServer` in `internal/config/config.go`.
  - Add field `Transport TransportConfig` to `UpstreamEndpoint` in `internal/config/upstream.go`.
  - Implement `mergeTransport(base TransportConfig, override *TransportConfig) TransportConfig` with zero-value inheritance (nil override returns base unchanged).
  - In `UpstreamEndpoints()`, set `ep.Transport = mergeTransport(c.Transport, s.Transport)` then inject the resolved per-server key into `ep.Transport.KCP.Key`; the legacy single `server.addr` path sets `Transport: c.Transport`.
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 2.1, 2.2, 2.3_

- [x] 2. Unit tests for merge and endpoint resolution
  - Add tests in `internal/config` covering: nil override returns base; partial override overlays only set fields; full override replaces; per-server key precedence over global key.
  - Test `UpstreamEndpoints` resolves and merges per-server transport, preserves priority ordering, and uses global transport for the legacy `server.addr` form.
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.1, 2.2_

- [x] 3. Refactor and extend transport validation
  - Extract the inline global-transport validation in `Config.Validate` into a reusable method `validateTransport(t *TransportConfig, label string) error` that fills defaults and validates protocol/conn/kcp/quic.
  - Call it for the global transport (label "transport"), and for each upstream server with a non-nil override validate the MERGED transport with a label naming the upstream.
  - Ensure errors name the offending upstream (e.g. `upstream "kharej-2": ...`).
  - _Requirements: 3.1, 3.2, 3.3_

- [x] 4. Unit tests for validation
  - Test that an invalid per-upstream protocol and invalid KCP params (negative shards, parity without data, shard sum > 255, negative windows) fail with an error naming the upstream.
  - Test that a valid mixed kcp/quic config passes and that a config with no overrides validates identically to before (backward compatibility).
  - _Requirements: 2.1, 3.1, 3.2, 3.3_

- [x] 5. Switch the upstream manager to use the resolved endpoint transport
  - In `internal/upstream/manager.go`, replace `cfg.TransportForKey(ep.Key)` with `ep.Transport` in `NewManager`, `Reload`, and `reconnect` (use `cur.ep.Transport`).
  - Leave `TransportForKey` in place for compatibility; no other manager logic changes.
  - _Requirements: 1.1, 1.2, 2.1_

- [x] 6. Fix the test command for range mode and report per-upstream protocol
  - In `internal/app/test.go`, replace the `forward/socks5 rules configured` check so it passes when range is enabled, and emit a `range mode: ...` line when range is on; keep failing when none of forward/socks5/range is configured.
  - When role is client and `cfg.Upstream != nil`, resolve endpoints and add one line per upstream reporting its resolved protocol.
  - Add unit tests for `TestConfig`: range-only client passes; client with none of the three still fails.
  - _Requirements: 3.4, 5.1, 5.2, 5.3_

- [x] 7. Installer wizard: per-upstream transport selection
  - In `install-premium.sh`, update `add_upstream_servers` to prompt a transport per upstream (default = the global choice), storing into a new `UP_TRANSPORTS` array.
  - In `write_client_config`, emit a nested `transport:` block under a server only when its transport differs from the global protocol; when all upstreams match, emit no per-server blocks.
  - Verify a generated mixed-transport config passes `paqetpremium test`.
  - _Requirements: 4.1, 4.2, 4.3, 4.4_

- [x] 8. Installer: range-mode strategy warning
  - In the client wizard, when range is enabled with more than one upstream and the strategy is not `failover`, print a warning that every exit server must expose the same target service on the configured target host.
  - _Requirements: 7.1_

- [x] 9. Kernel RST suppression in systemd units and installer
  - Generate per-unit `ExecStartPre=` (idempotent `iptables -C ... || iptables -A ...`) and `ExecStopPost=` (`iptables -D ...`) rules: server uses `OUTPUT -p tcp --sport <listen_port> --tcp-flags RST RST -j DROP -m comment --comment paqetpremium`; client uses `OUTPUT -p tcp --dport <upstream_port> --tcp-flags RST RST -j DROP ...` for each upstream port.
  - Mirror with `ip6tables` when IPv6 is configured.
  - Update `cmd_uninstall` to sweep leftover rules tagged with the `paqetpremium` comment.
  - _Requirements: 6.1, 6.2, 6.3, 6.4_

- [x] 10. Documentation and changelog
  - Update `README.md` and `README.fa.md`: per-upstream transport YAML example (mixed kcp/quic) with the zero-value inheritance note; RST suppression explanation plus manual iptables commands for prebuilt-binary users; failover-vs-load-balancing guidance for range mode.
  - Add CHANGELOG `[Unreleased]` entries for per-upstream transport, the range test fix, and RST suppression.
  - _Requirements: 6.5, 7.2_

- [x] 11. Final verification
  - Run `go build ./...`, `go vet ./...`, and `go test ./...`; fix any issues.
  - Confirm `gofmt -l` is clean for changed Go files.
  - _Requirements: 1.1, 2.1, 3.1, 5.1, 6.1_
