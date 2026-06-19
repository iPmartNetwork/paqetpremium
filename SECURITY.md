# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities **privately**. Do not open public issues,
pull requests, or discussions for security problems, as that exposes users
before a fix is available.

The preferred channel is GitHub Security Advisories:
https://github.com/iPmartNetwork/paqetpremium/security/advisories/new

When reporting, please include:

- The output of `paqetpremium version`.
- The affected component (e.g. pcap engine, KCP/QUIC transport, SOCKS5, admin
  API, installer, range mode).
- Clear steps to reproduce, and the impact you observed.

Never include real keys, tokens, shared secrets, or other credentials in a
report. Redact them or use placeholders.

Maintainers aim to acknowledge reports within a few days. Please allow time for
a fix to be prepared and released before any public disclosure.

## Supported versions

Security fixes target the **latest released minor** version. If you are running
an older build, please upgrade to the latest release before reporting, since the
issue may already be fixed.

## Hardening notes

- Use a **strong, unique shared secret** for every deployment. The secret keys
  both the KCP path and (deterministically) the QUIC certificate pinning, so a
  weak or reused secret undermines peer authentication.
- The admin API binds to `127.0.0.1` by default. If you expose it on a routable
  interface, set `admin.token` and treat that token as a credential.
- All-ports range mode exposes the server's **local ports** to clients via the
  entry IP. Keep sensitive ports (databases, management interfaces, etc.) in the
  `exclude` list so they are not reachable through the tunnel.
