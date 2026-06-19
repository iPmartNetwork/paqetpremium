# Contributing to PaqetPremium

Thanks for your interest in improving PaqetPremium. This guide covers how to set
up a development environment, build and test the project, and submit changes.

## Development setup

- **Go 1.25+** is required.
- The packet engine uses **CGO + libpcap** and only fully builds on **Linux**.
  On a Debian/Ubuntu host:

  ```sh
  sudo apt-get install -y libpcap-dev
  CGO_ENABLED=1 go build ./...
  ```

- On non-Linux platforms (macOS, Windows), the Linux-only files are excluded by
  build tags, so `go build ./...` still compiles the cross-platform packages.
  You won't be able to exercise the pcap engine there, but most logic and the
  loopback integration tests still build and run.

## Build, test, and lint

```sh
go build ./...      # compile all packages
go vet ./...        # static checks
go test ./...       # run the test suite
gofmt -l .          # should print nothing (no unformatted files)
```

The loopback integration tests in `internal/tunnel` and `internal/tunnelpool`
exercise a full client<->server tunnel over loopback PacketConns and need **no
pcap engine and no root**, so they run on any platform and in CI.

## Pull requests

- Branch from `master` and keep each PR focused on a single change.
- Before opening a PR, run `go build ./...`, `go vet ./...`, `go test ./...`,
  and make sure `gofmt -l .` is empty.
- Add an entry to `CHANGELOG.md` under `## [Unreleased]`.
- Keep `README.md` and `README.fa.md` in sync when you change user-visible
  behavior.
- Do not commit secrets or compiled binaries.

## Releases

Pushing a `vX.Y.Z` tag triggers CI to build the Linux amd64 + arm64 binaries and
`.deb`/`.rpm` packages and publish a GitHub Release. Regular contributors do not
need to create tags.

## Security issues

Please do not report security vulnerabilities in public issues or PRs. Follow
[SECURITY.md](SECURITY.md) and report them privately.
