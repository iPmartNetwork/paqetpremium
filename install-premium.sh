#!/usr/bin/env bash
#
# PaqetPremium — installer & manager for Linux VPS
# Repository : https://github.com/iPmartNetwork/paqetpremium  (branch: master)
#
# Packet-level tunnel (libpcap + KCP/QUIC + smux):
#   * Iran VPS    -> role: client  (entry: port-forward / SOCKS5)
#   * Kharej VPS  -> role: server  (exit: relay + iptables)
#
# Usage:
#   sudo ./install-premium.sh              # interactive menu
#   sudo ./install-premium.sh server       # guided server (Kharej) setup
#   sudo ./install-premium.sh client       # guided client (Iran) setup
#   sudo ./install-premium.sh add-tunnel   # add a named client instance
#   sudo ./install-premium.sh status|list|logs|reload|restart|update|uninstall
#
set -uo pipefail

# --------------------------------------------------------------------------- #
# Constants
# --------------------------------------------------------------------------- #
readonly APP_NAME="PaqetPremium"
readonly BIN_NAME="paqetpremium"
readonly BIN_DIR="/usr/local/bin"
readonly CONFIG_DIR="/etc/paqetpremium"
readonly TUNNELS_DIR="${CONFIG_DIR}/tunnels"
readonly SERVICE_DIR="/etc/systemd/system"
readonly SRC_DIR="/opt/paqetpremium/src"

readonly REPO_URL="https://github.com/iPmartNetwork/paqetpremium"
readonly REPO_BRANCH="master"
readonly REPO_RAW="https://raw.githubusercontent.com/iPmartNetwork/paqetpremium/master"

readonly GO_MIN="1.25"
readonly GO_DL_VERSION="1.25.4"

# Install behaviour (override via environment)
INSTALL_MODE="${INSTALL_MODE:-auto}"        # auto | binary | source
RELEASE_VERSION="${RELEASE_VERSION:-latest}" # latest | vX.Y.Z (for prebuilt installs)

# --------------------------------------------------------------------------- #
# Pretty output
# --------------------------------------------------------------------------- #
if [[ -t 1 ]]; then
  C_RED=$'\033[0;31m'; C_GRN=$'\033[0;32m'; C_YLW=$'\033[1;33m'
  C_CYN=$'\033[0;36m'; C_DIM=$'\033[2m'; C_BLD=$'\033[1m'; C_NC=$'\033[0m'
else
  C_RED=""; C_GRN=""; C_YLW=""; C_CYN=""; C_DIM=""; C_BLD=""; C_NC=""
fi

info() { printf '%s[i]%s %s\n' "$C_CYN" "$C_NC" "$*"; }
ok()   { printf '%s[OK]%s %s\n' "$C_GRN" "$C_NC" "$*"; }
warn() { printf '%s[!]%s %s\n' "$C_YLW" "$C_NC" "$*" >&2; }
err()  { printf '%s[x]%s %s\n' "$C_RED" "$C_NC" "$*" >&2; exit 1; }
hr()   { printf '%s%s%s\n' "$C_DIM" "------------------------------------------------------------" "$C_NC"; }

# Note: this interactive installer deliberately does NOT use `set -e` or an ERR
# trap. Many routine commands (network detection, `awk ... exit` pipelines that
# raise SIGPIPE under pipefail, `systemctl is-active`, `grep` with no match)
# legitimately return non-zero. Critical operations handle failure explicitly
# with `|| err`.

banner() {
  printf '%s\n' "$C_BLD"
  cat <<'ART'
   ___                  _   ___                  _
  | _ \__ _ __ _ ___ __| |_| _ \_ _ ___ _ __ (_)_  _ _ __
  |  _/ _` / _` / -_) _|  _|  _/ '_/ -_) '  \| | || | '  \
  |_| \__,_\__, \___\__|\__|_| |_| \___|_|_|_|_|\_,_|_|_|_|
           |___/        packet-level tunnel for Linux VPS
ART
  printf '%s' "$C_NC"
}

# --------------------------------------------------------------------------- #
# Pre-flight checks
# --------------------------------------------------------------------------- #
require_root()    { [[ ${EUID} -eq 0 ]] || err "Please run as root (sudo)."; }
require_linux()   { [[ "$(uname -s)" == "Linux" ]] || err "${APP_NAME} runs on Linux only."; }
require_systemd() { command -v systemctl >/dev/null 2>&1 || err "systemd (systemctl) is required."; }

need_cmd() { command -v "$1" >/dev/null 2>&1; }

PKG=""
detect_pkg_mgr() {
  if   need_cmd apt-get; then PKG="apt"
  elif need_cmd dnf;     then PKG="dnf"
  elif need_cmd yum;     then PKG="yum"
  elif need_cmd pacman;  then PKG="pacman"
  elif need_cmd zypper;  then PKG="zypper"
  else PKG=""; fi
}

pkg_install() {
  case "$PKG" in
    apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y "$@" ;;
    dnf)    dnf install -y "$@" ;;
    yum)    yum install -y "$@" ;;
    pacman) pacman -Sy --noconfirm "$@" ;;
    zypper) zypper install -y "$@" ;;
    *)      warn "Unknown package manager; ensure these are installed: $*" ;;
  esac
}

install_deps() {
  info "Installing build & runtime dependencies..."
  detect_pkg_mgr
  case "$PKG" in
    apt)    apt-get update -qq
            pkg_install ca-certificates curl git iptables build-essential libpcap-dev ;;
    dnf|yum) pkg_install ca-certificates curl git iptables gcc make libpcap-devel ;;
    pacman) pkg_install ca-certificates curl git iptables base-devel libpcap ;;
    zypper) pkg_install ca-certificates curl git iptables gcc make libpcap-devel ;;
    *)      warn "Install manually: curl git iptables a C toolchain and libpcap headers." ;;
  esac
  ok "Dependencies ready."
}

# --------------------------------------------------------------------------- #
# Go toolchain (build requires CGO + libpcap)
# --------------------------------------------------------------------------- #
version_ge() { [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" == "$2" ]]; }

go_bin() { command -v go 2>/dev/null || echo "/usr/local/go/bin/go"; }

ensure_go() {
  local go; go="$(go_bin)"
  if [[ -x "$go" ]] || need_cmd go; then
    local v; v="$("$(go_bin)" version 2>/dev/null | awk '{print $3}' | sed 's/^go//')"
    if [[ -n "$v" ]] && version_ge "$v" "$GO_MIN"; then
      ok "Go ${v} detected."
      return
    fi
    warn "Go ${v:-unknown} is older than required ${GO_MIN}; fetching ${GO_DL_VERSION}."
  else
    warn "Go toolchain not found; fetching ${GO_DL_VERSION}."
  fi
  install_go_tarball
}

install_go_tarball() {
  local arch tar url
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) err "Unsupported CPU architecture: $(uname -m) (need amd64/arm64)." ;;
  esac
  tar="go${GO_DL_VERSION}.linux-${arch}.tar.gz"
  url="https://go.dev/dl/${tar}"
  info "Downloading ${url}"
  curl -fsSL "$url" -o "/tmp/${tar}" || err "failed to download Go from ${url}"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "/tmp/${tar}" || err "failed to extract Go toolchain"
  rm -f "/tmp/${tar}"
  export PATH="/usr/local/go/bin:${PATH}"
  ok "Go $("/usr/local/go/bin/go" version | awk '{print $3}') installed to /usr/local/go."
}

# --------------------------------------------------------------------------- #
# Source acquisition + build
# --------------------------------------------------------------------------- #
SRC=""
acquire_source() {
  if [[ -f "./go.mod" ]] && grep -q "paqetpremium" "./go.mod" 2>/dev/null; then
    SRC="$(pwd)"
    info "Using source in current directory: ${SRC}"
    return
  fi
  need_cmd git || { detect_pkg_mgr; pkg_install git; }
  if [[ -d "${SRC_DIR}/.git" ]]; then
    info "Updating existing source (${REPO_BRANCH})..."
    git -C "${SRC_DIR}" fetch --depth 1 origin "${REPO_BRANCH}" || err "git fetch failed"
    git -C "${SRC_DIR}" reset --hard "origin/${REPO_BRANCH}" || err "git reset failed"
  else
    info "Cloning ${REPO_URL} (${REPO_BRANCH})..."
    mkdir -p "$(dirname "${SRC_DIR}")"
    git clone --depth 1 --branch "${REPO_BRANCH}" "${REPO_URL}" "${SRC_DIR}" || err "git clone failed"
  fi
  SRC="${SRC_DIR}"
}

build_binary() {
  ensure_go
  acquire_source
  local go; go="$(go_bin)"
  info "Building ${BIN_NAME} (CGO + libpcap)..."
  if ! ( cd "${SRC}" && CGO_ENABLED=1 "$go" build -trimpath -ldflags "-s -w" -o "/tmp/${BIN_NAME}" ./cmd/paqetpremium ); then
    err "build failed (ensure libpcap-dev and a C toolchain are installed)"
  fi
  install -m 0755 "/tmp/${BIN_NAME}" "${BIN_DIR}/${BIN_NAME}" || err "failed to install binary to ${BIN_DIR}"
  rm -f "/tmp/${BIN_NAME}"
  ok "Installed: ${BIN_DIR}/${BIN_NAME} ($(${BIN_DIR}/${BIN_NAME} version 2>/dev/null || echo "${BIN_NAME}"))"
}

# Lightweight runtime dependencies for prebuilt-binary installs.
install_runtime_deps() {
  info "Installing runtime dependencies..."
  detect_pkg_mgr
  case "$PKG" in
    apt)     apt-get update -qq; pkg_install ca-certificates curl iptables libpcap0.8 ;;
    dnf|yum) pkg_install ca-certificates curl iptables libpcap ;;
    pacman)  pkg_install ca-certificates curl iptables libpcap ;;
    zypper)  pkg_install ca-certificates curl iptables libpcap1 ;;
    *)       warn "Ensure curl, iptables and the libpcap runtime are installed." ;;
  esac
  ok "Runtime dependencies ready."
}

# Download a prebuilt release binary and verify its checksum. Returns non-zero on failure.
download_release_binary() {
  local arch ver base asset tmp
  case "$(uname -m)" in
    x86_64|amd64)  arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) warn "No prebuilt binary for $(uname -m)."; return 1 ;;
  esac
  ver="${RELEASE_VERSION:-latest}"
  if [[ "$ver" == "latest" ]]; then
    base="${REPO_URL}/releases/latest/download"
  else
    base="${REPO_URL}/releases/download/${ver}"
  fi
  asset="${BIN_NAME}-linux-${arch}"
  tmp="$(mktemp -d)"
  info "Fetching prebuilt ${asset} (${ver})..."
  if ! curl -fsSL "${base}/${asset}" -o "${tmp}/${asset}"; then
    rm -rf "$tmp"; return 1
  fi
  if need_cmd sha256sum && curl -fsSL "${base}/${asset}.sha256" -o "${tmp}/${asset}.sha256" 2>/dev/null; then
    if ( cd "$tmp" && sha256sum -c "${asset}.sha256" >/dev/null 2>&1 ); then
      ok "Checksum verified."
    else
      warn "Checksum mismatch; discarding download."
      rm -rf "$tmp"; return 1
    fi
  else
    warn "Checksum unavailable; skipping verification."
  fi
  if ! install -m 0755 "${tmp}/${asset}" "${BIN_DIR}/${BIN_NAME}"; then
    rm -rf "$tmp"; return 1
  fi
  rm -rf "$tmp"
  ok "Installed prebuilt: ${BIN_DIR}/${BIN_NAME} ($(${BIN_DIR}/${BIN_NAME} version 2>/dev/null || echo "${BIN_NAME}"))"
}

# Provision the binary: prefer a prebuilt release (with source fallback) unless overridden.
provision_binary() {
  if [[ "${INSTALL_MODE}" != "source" ]]; then
    install_runtime_deps
    if download_release_binary; then
      return
    fi
    if [[ "${INSTALL_MODE}" == "binary" ]]; then
      err "Prebuilt binary unavailable and INSTALL_MODE=binary."
    fi
    warn "No prebuilt binary available; building from source instead."
  fi
  install_deps
  build_binary
}

# --------------------------------------------------------------------------- #
# Network auto-detection
# --------------------------------------------------------------------------- #
detect_iface()  { ip route 2>/dev/null | awk '/^default/ {print $5; exit}'; }
detect_ipv4()   { ip -4 addr show "${1:-}" 2>/dev/null | awk '/inet / {print $2; exit}' | cut -d/ -f1; }
detect_ipv6()   { ip -6 addr show "${1:-}" scope global 2>/dev/null | awk '/inet6/ {print $2; exit}' | cut -d/ -f1; }
detect_gw_mac() {
  local gw; gw="$(ip route 2>/dev/null | awk '/^default/ {print $3; exit}')"
  [[ -n "$gw" ]] || return 0
  ping -c1 -W1 "$gw" >/dev/null 2>&1 || true
  ip neigh show "$gw" 2>/dev/null | awk '{print $5; exit}'
}

gen_secret() {
  if need_cmd openssl; then openssl rand -hex 16
  else tr -dc 'a-f0-9' </dev/urandom | head -c 32; echo; fi
}

prompt() {  # prompt <var> <question> [default]
  local __var="$1" __q="$2" __def="${3:-}" __ans=""
  if [[ -n "$__def" ]]; then
    read -rp "$(printf '%s%s%s [%s]: ' "$C_BLD" "$__q" "$C_NC" "$__def")" __ans </dev/tty || true
    __ans="${__ans:-$__def}"
  else
    read -rp "$(printf '%s%s%s: ' "$C_BLD" "$__q" "$C_NC")" __ans </dev/tty || true
  fi
  printf -v "$__var" '%s' "$__ans"
}

# --------------------------------------------------------------------------- #
# Config writers (driven by CFG_* globals)
# --------------------------------------------------------------------------- #
CFG_NAME=""; CFG_IFACE=""; CFG_IPV4=""; CFG_IPV6=""; CFG_RMAC=""; CFG_RMAC6=""
CFG_PORT=""; CFG_KEY=""; CFG_TRANSPORT="kcp"; CFG_CONN="6"
CFG_SERVER_ADDR=""; CFG_STRATEGY="failover"; CFG_FWD_PORTS=""; CFG_SOCKS_PORT=""
CFG_SOCKS_BIND="127.0.0.1"; CFG_ADMIN="127.0.0.1:9090"; CFG_TOKEN=""

emit_transport() {
  echo "transport:"
  echo "  protocol: ${CFG_TRANSPORT}"
  echo "  conn: ${CFG_CONN}"
  if [[ "${CFG_TRANSPORT}" == "quic" ]]; then
    echo "  kcp:"
    echo "    key: ${CFG_KEY}"
    echo "  quic:"
    echo "    alpn: paqetpremium"
    echo "    idle_timeout: 30s"
    echo "    max_idle_timeout: 60s"
  else
    echo "  kcp:"
    echo "    mode: fast"
    echo "    block: aes-128-gcm"
    echo "    key: ${CFG_KEY}"
    echo "    mtu: 1150"
  fi
}

emit_admin() {
  echo "admin:"
  echo "  listen: \"${CFG_ADMIN}\""
  echo "  metrics: true"
  [[ -n "${CFG_TOKEN}" ]] && echo "  token: ${CFG_TOKEN}"
}

write_server_config() {
  local cfg="$1"
  mkdir -p "$(dirname "$cfg")"
  {
    echo "core: paqetpremium"
    echo "version: 2"
    echo "role: server"
    echo "name: ${CFG_NAME:-kharej-exit}"
    echo
    echo "log:"
    echo "  level: info"
    echo
    echo "listen:"
    echo "  addr: \":${CFG_PORT}\""
    echo
    echo "network:"
    echo "  interface: ${CFG_IFACE}"
    echo "  ipv4:"
    echo "    addr: \"${CFG_IPV4}:${CFG_PORT}\""
    echo "    router_mac: \"${CFG_RMAC}\""
    if [[ -n "${CFG_IPV6}" ]]; then
      echo "  ipv6:"
      echo "    addr: \"[${CFG_IPV6}]:${CFG_PORT}\""
      echo "    router_mac: \"${CFG_RMAC6:-$CFG_RMAC}\""
    fi
    echo "  tcp:"
    echo "    local_flag: [\"PA\"]"
    echo
    emit_transport
    echo
    emit_admin
  } >"$cfg"
  chmod 0640 "$cfg"
  ok "Server config written: ${cfg}"
}

write_client_config() {
  local cfg="$1"
  mkdir -p "$(dirname "$cfg")"
  {
    echo "core: paqetpremium"
    echo "version: 2"
    echo "role: client"
    echo "name: ${CFG_NAME:-iran-entry}"
    echo
    echo "log:"
    echo "  level: info"
    echo
    echo "upstream:"
    echo "  strategy: ${CFG_STRATEGY}"
    echo "  health_check:"
    echo "    interval: 10s"
    echo "    timeout: 3s"
    echo "    fail_threshold: 3"
    echo "    recover_threshold: 2"
    echo "  servers:"
    echo "    - name: primary"
    echo "      addr: ${CFG_SERVER_ADDR}"
    echo "      key: ${CFG_KEY}"
    echo "      priority: 1"
    echo "      weight: 1"
    echo
    if [[ -n "${CFG_FWD_PORTS}" ]]; then
      echo "forward:"
      local p; IFS=',' read -ra _ports <<<"${CFG_FWD_PORTS}"
      for p in "${_ports[@]}"; do
        p="${p// /}"; [[ -n "$p" ]] || continue
        echo "  - listen: \"0.0.0.0:${p}\""
        echo "    target: \"127.0.0.1:${p}\""
        echo "    protocol: tcp"
      done
      echo
    fi
    if [[ -n "${CFG_SOCKS_PORT}" ]]; then
      echo "socks5:"
      echo "  - listen: \"${CFG_SOCKS_BIND}:${CFG_SOCKS_PORT}\""
      echo
    fi
    echo "network:"
    echo "  interface: ${CFG_IFACE}"
    echo "  ipv4:"
    echo "    addr: \"${CFG_IPV4}:0\""
    echo "    router_mac: \"${CFG_RMAC}\""
    if [[ -n "${CFG_IPV6}" ]]; then
      echo "  ipv6:"
      echo "    addr: \"[${CFG_IPV6}]:0\""
      echo "    router_mac: \"${CFG_RMAC6:-$CFG_RMAC}\""
    fi
    echo "  tcp:"
    echo "    local_flag: [\"PA\"]"
    echo "    remote_flag: [\"PA\"]"
    echo
    emit_transport
    echo
    emit_admin
  } >"$cfg"
  chmod 0640 "$cfg"
  ok "Client config written: ${cfg}"
}

# --------------------------------------------------------------------------- #
# systemd units
# --------------------------------------------------------------------------- #
write_unit() {  # write_unit <unit-file> <description> <config-path>
  local unit="$1" desc="$2" cfg="$3"
  cat >"${SERVICE_DIR}/${unit}" <<EOF
[Unit]
Description=${desc}
Documentation=${REPO_URL}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_DIR}/${BIN_NAME} run -c ${cfg}
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN
NoNewPrivileges=false

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
}

install_service() {  # install_service <server|client>
  local role="$1"
  write_unit "${BIN_NAME}-${role}.service" "${APP_NAME} ${role}" "${CONFIG_DIR}/${role}.yaml"
  systemctl enable "${BIN_NAME}-${role}.service" >/dev/null 2>&1 || true
  ok "Installed unit: ${BIN_NAME}-${role}.service"
}

install_client_template() {
  cat >"${SERVICE_DIR}/${BIN_NAME}-client@.service" <<EOF
[Unit]
Description=${APP_NAME} client instance %i
Documentation=${REPO_URL}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_DIR}/${BIN_NAME} run -c ${TUNNELS_DIR}/%i.yaml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN
NoNewPrivileges=false

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  ok "Installed template unit: ${BIN_NAME}-client@.service"
}

healthcheck() {
  local listen="${1:-$CFG_ADMIN}"
  need_cmd curl || return 0
  sleep 1
  if curl -fsS --max-time 3 "http://${listen}/healthz" >/dev/null 2>&1; then
    ok "Admin healthcheck passed (http://${listen}/healthz)."
  else
    warn "Admin healthcheck did not respond yet; check logs with: journalctl -u <unit> -f"
  fi
}

# --------------------------------------------------------------------------- #
# Wizards
# --------------------------------------------------------------------------- #
ask_transport() {
  local t
  prompt t "Transport protocol (kcp/quic)" "${CFG_TRANSPORT}"
  case "${t,,}" in
    quic) CFG_TRANSPORT="quic" ;;
    *)    CFG_TRANSPORT="kcp" ;;
  esac
}

ask_common_network() {
  local d_iface d_ipv4 d_ipv6 d_mac
  d_iface="$(detect_iface)"; d_ipv4="$(detect_ipv4 "$d_iface")"
  d_ipv6="$(detect_ipv6 "$d_iface")"; d_mac="$(detect_gw_mac)"
  prompt CFG_IFACE "Network interface" "${d_iface}"
  prompt CFG_IPV4  "Local IPv4 address" "${d_ipv4}"
  prompt CFG_RMAC  "Gateway/router MAC" "${d_mac}"
  local want6="n"
  [[ -n "$d_ipv6" ]] && want6="y"
  prompt want6 "Enable IPv6? (y/n)" "${want6}"
  if [[ "${want6,,}" == "y" ]]; then
    prompt CFG_IPV6  "Local IPv6 address" "${d_ipv6}"
    prompt CFG_RMAC6 "IPv6 gateway MAC" "${d_mac}"
  else
    CFG_IPV6=""; CFG_RMAC6=""
  fi
}

wizard_server() {
  require_root; require_linux; require_systemd
  banner; hr; info "Server (Kharej / exit) setup"; hr
  provision_binary
  ask_common_network
  prompt CFG_PORT "Tunnel listen port" "8888"
  ask_transport
  CFG_KEY="$(gen_secret)"
  prompt CFG_KEY "Shared secret key (use the SAME on the client)" "${CFG_KEY}"
  prompt CFG_ADMIN "Admin API listen" "127.0.0.1:9090"
  prompt CFG_TOKEN "Admin API token (blank = none)" ""
  CFG_NAME="kharej-exit"
  write_server_config "${CONFIG_DIR}/server.yaml"
  "${BIN_DIR}/${BIN_NAME}" test -c "${CONFIG_DIR}/server.yaml" || warn "Config test reported issues (live checks need a running peer)."
  install_service server
  systemctl restart "${BIN_NAME}-server.service" || warn "service did not start cleanly; inspect: journalctl -u ${BIN_NAME}-server -e"
  healthcheck "${CFG_ADMIN}"
  hr
  ok "Server is up. Secret key: ${C_BLD}${CFG_KEY}${C_NC}"
  info "Manage: systemctl status ${BIN_NAME}-server | journalctl -u ${BIN_NAME}-server -f"
}

wizard_client() {
  require_root; require_linux; require_systemd
  banner; hr; info "Client (Iran / entry) setup"; hr
  provision_binary
  ask_common_network
  prompt CFG_SERVER_ADDR "Server address (host:port)" ""
  [[ -n "${CFG_SERVER_ADDR}" ]] || err "Server address is required."
  ask_transport
  prompt CFG_KEY "Shared secret key (SAME as server)" ""
  [[ -n "${CFG_KEY}" ]] || err "Shared secret key is required."
  prompt CFG_FWD_PORTS "Forward TCP ports (comma-separated, blank = none)" "443,8443"
  prompt CFG_SOCKS_PORT "SOCKS5 listen port (blank = disabled)" "1080"
  [[ -z "${CFG_FWD_PORTS}" && -z "${CFG_SOCKS_PORT}" ]] && err "Provide at least a forward port or a SOCKS5 port."
  prompt CFG_ADMIN "Admin API listen" "127.0.0.1:9090"
  prompt CFG_TOKEN "Admin API token (blank = none)" ""
  CFG_NAME="iran-entry"
  write_client_config "${CONFIG_DIR}/client.yaml"
  "${BIN_DIR}/${BIN_NAME}" test -c "${CONFIG_DIR}/client.yaml" || warn "Config test reported issues (live checks need a reachable server)."
  install_service client
  systemctl restart "${BIN_NAME}-client.service" || warn "service did not start cleanly; inspect: journalctl -u ${BIN_NAME}-client -e"
  healthcheck "${CFG_ADMIN}"
  hr
  ok "Client is up."
  info "Manage: systemctl status ${BIN_NAME}-client | journalctl -u ${BIN_NAME}-client -f"
}

wizard_add_tunnel() {
  require_root; require_linux; require_systemd
  [[ -x "${BIN_DIR}/${BIN_NAME}" ]] || provision_binary
  install_client_template
  local name
  prompt name "Tunnel instance name (e.g. v2ray-1)" ""
  [[ -n "$name" ]] || err "Instance name is required."
  ask_common_network
  prompt CFG_SERVER_ADDR "Server address (host:port)" ""
  [[ -n "${CFG_SERVER_ADDR}" ]] || err "Server address is required."
  ask_transport
  prompt CFG_KEY "Shared secret key" ""
  [[ -n "${CFG_KEY}" ]] || err "Shared secret key is required."
  prompt CFG_FWD_PORTS "Forward TCP ports (comma-separated, blank = none)" "443"
  prompt CFG_SOCKS_PORT "SOCKS5 listen port (blank = disabled)" ""
  [[ -z "${CFG_FWD_PORTS}" && -z "${CFG_SOCKS_PORT}" ]] && err "Provide at least a forward port or a SOCKS5 port."
  prompt CFG_ADMIN "Admin API listen" "127.0.0.1:9090"
  prompt CFG_TOKEN "Admin API token (blank = none)" ""
  CFG_NAME="$name"
  write_client_config "${TUNNELS_DIR}/${name}.yaml"
  systemctl enable "${BIN_NAME}-client@${name}.service" >/dev/null 2>&1 || true
  systemctl restart "${BIN_NAME}-client@${name}.service" || warn "instance did not start cleanly; inspect: journalctl -u ${BIN_NAME}-client@${name} -e"
  ok "Tunnel '${name}' started (${BIN_NAME}-client@${name})."
}

# --------------------------------------------------------------------------- #
# Management
# --------------------------------------------------------------------------- #
list_units() {
  hr; info "${APP_NAME} services"; hr
  local found=0 u st
  for u in "${BIN_NAME}-server.service" "${BIN_NAME}-client.service"; do
    if systemctl list-unit-files --no-legend "$u" 2>/dev/null | grep -q .; then
      st="$(systemctl is-active "$u" 2>/dev/null || true)"
      printf '  %-34s %s\n' "$u" "$st"; found=1
    fi
  done
  if [[ -d "${TUNNELS_DIR}" ]]; then
    local f base
    for f in "${TUNNELS_DIR}"/*.yaml; do
      [[ -e "$f" ]] || continue
      base="$(basename "$f" .yaml)"
      st="$(systemctl is-active "${BIN_NAME}-client@${base}.service" 2>/dev/null || true)"
      printf '  %-34s %s\n' "${BIN_NAME}-client@${base}" "$st"; found=1
    done
  fi
  [[ "$found" -eq 1 ]] || info "No services installed yet."
}

cmd_status() {
  list_units
  if [[ -x "${BIN_DIR}/${BIN_NAME}" ]] && need_cmd curl; then
    local listen
    listen="$(awk '/^admin:/{a=1} a&&/listen:/{gsub(/[ "\047]/,"",$2);print $2;exit}' "${CONFIG_DIR}/server.yaml" "${CONFIG_DIR}/client.yaml" 2>/dev/null | head -1)"
    [[ -n "${listen:-}" ]] && curl -fsS --max-time 3 "http://${listen}/api/v1/status" 2>/dev/null || true
  fi
}

resolve_unit() {  # echoes a concrete unit for an optional <name> arg
  local arg="${1:-}"
  if [[ -z "$arg" ]]; then
    if systemctl list-unit-files --no-legend "${BIN_NAME}-server.service" 2>/dev/null | grep -q .; then
      echo "${BIN_NAME}-server.service"
    else
      echo "${BIN_NAME}-client.service"
    fi
  elif [[ "$arg" == "server" || "$arg" == "client" ]]; then
    echo "${BIN_NAME}-${arg}.service"
  else
    echo "${BIN_NAME}-client@${arg}.service"
  fi
}

cmd_logs()    { require_systemd; journalctl -u "$(resolve_unit "${1:-}")" -f --no-pager; }
cmd_restart() { require_root; require_systemd; local u; u="$(resolve_unit "${1:-}")"; systemctl restart "$u"; ok "Restarted ${u}"; }
cmd_reload()  { require_root; require_systemd; local u; u="$(resolve_unit "${1:-}")"; systemctl reload "$u" && ok "Reloaded ${u} (SIGHUP)"; }

cmd_update() {
  require_root; require_linux
  info "Updating ${BIN_NAME} (mode: ${INSTALL_MODE})..."
  provision_binary
  require_systemd
  local u
  for u in "${BIN_NAME}-server.service" "${BIN_NAME}-client.service"; do
    systemctl is-active "$u" >/dev/null 2>&1 && { systemctl restart "$u"; ok "Restarted ${u}"; }
  done
  if [[ -d "${TUNNELS_DIR}" ]]; then
    local f base
    for f in "${TUNNELS_DIR}"/*.yaml; do
      [[ -e "$f" ]] || continue
      base="$(basename "$f" .yaml)"
      systemctl is-active "${BIN_NAME}-client@${base}.service" >/dev/null 2>&1 && \
        { systemctl restart "${BIN_NAME}-client@${base}.service"; ok "Restarted ${BIN_NAME}-client@${base}"; }
    done
  fi
  ok "Update complete."
}

cmd_uninstall() {
  require_root; require_systemd
  warn "This stops and removes all ${APP_NAME} services and the binary."
  local yes="n"; prompt yes "Proceed? (y/n)" "n"
  [[ "${yes,,}" == "y" ]] || { info "Cancelled."; return; }
  local u
  for u in "${BIN_NAME}-server.service" "${BIN_NAME}-client.service"; do
    systemctl disable --now "$u" >/dev/null 2>&1 || true
    rm -f "${SERVICE_DIR}/${u}"
  done
  if [[ -d "${TUNNELS_DIR}" ]]; then
    local f base
    for f in "${TUNNELS_DIR}"/*.yaml; do
      [[ -e "$f" ]] || continue
      base="$(basename "$f" .yaml)"
      systemctl disable --now "${BIN_NAME}-client@${base}.service" >/dev/null 2>&1 || true
    done
  fi
  rm -f "${SERVICE_DIR}/${BIN_NAME}-client@.service"
  systemctl daemon-reload
  rm -f "${BIN_DIR}/${BIN_NAME}"
  ok "Binary and services removed."
  local delcfg="n"; prompt delcfg "Also delete ${CONFIG_DIR}? (y/n)" "n"
  [[ "${delcfg,,}" == "y" ]] && { rm -rf "${CONFIG_DIR}"; ok "Removed ${CONFIG_DIR}"; }
}

# --------------------------------------------------------------------------- #
# Menu + dispatch
# --------------------------------------------------------------------------- #
menu() {
  banner
  cat <<EOF

  ${C_BLD}${APP_NAME}${C_NC} — installer & manager  ${C_DIM}(${REPO_BRANCH})${C_NC}

   1) Install / configure  ${C_DIM}server  (Kharej / exit)${C_NC}
   2) Install / configure  ${C_DIM}client  (Iran / entry)${C_NC}
   3) Add named client tunnel (multi-instance)
   4) Status
   5) Follow logs
   6) Reload config (SIGHUP)
   7) Restart service
   8) Update (rebuild from repo)
   9) Uninstall
   0) Exit

EOF
  local c; prompt c "Select" ""
  case "$c" in
    1) wizard_server ;;
    2) wizard_client ;;
    3) wizard_add_tunnel ;;
    4) cmd_status ;;
    5) local n; prompt n "Unit (server/client/<tunnel>, blank=auto)" ""; cmd_logs "$n" ;;
    6) local n; prompt n "Unit (server/client/<tunnel>, blank=auto)" ""; cmd_reload "$n" ;;
    7) local n; prompt n "Unit (server/client/<tunnel>, blank=auto)" ""; cmd_restart "$n" ;;
    8) cmd_update ;;
    9) cmd_uninstall ;;
    0) exit 0 ;;
    *) warn "Invalid selection." ;;
  esac
}

usage() {
  cat <<EOF
${APP_NAME} installer & manager

Usage: sudo $0 [command] [arg]

Commands:
  (none)            Interactive menu
  server            Guided server (Kharej) setup
  client            Guided client (Iran) setup
  add-tunnel        Add a named client instance
  status            Show services and admin status
  list              List installed services
  logs   [unit]     Follow logs (unit: server|client|<tunnel>)
  reload [unit]     Reload config via SIGHUP
  restart[unit]     Restart a service
  update            Rebuild from ${REPO_URL} and restart services
  uninstall         Remove services and binary
  help              Show this help

Environment:
  INSTALL_MODE     auto (default) | binary | source
  RELEASE_VERSION  latest (default) | vX.Y.Z  (prebuilt installs)

Repository: ${REPO_URL} (branch: ${REPO_BRANCH})
EOF
}

main() {
  local cmd="${1:-menu}"; shift || true
  case "$cmd" in
    menu)        require_root; require_linux; require_systemd; while true; do menu; printf '\n'; read -rp "Press Enter to continue..." </dev/tty || true; done ;;
    server)      wizard_server ;;
    client)      wizard_client ;;
    add-tunnel)  wizard_add_tunnel ;;
    status)      cmd_status ;;
    list)        list_units ;;
    logs)        cmd_logs "${1:-}" ;;
    reload)      cmd_reload "${1:-}" ;;
    restart)     cmd_restart "${1:-}" ;;
    update)      cmd_update ;;
    uninstall)   cmd_uninstall ;;
    help|-h|--help) usage ;;
    *)           usage; err "Unknown command: ${cmd}" ;;
  esac
}

main "$@"
