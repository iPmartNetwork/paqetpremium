#!/usr/bin/env bash
#
# PaqetPremium - installer & manager for Linux VPS
# Repository : https://github.com/iPmartNetwork/paqetpremium  (branch: master)
#
# Packet-level tunnel (libpcap + KCP/QUIC + smux):
#   * Iran VPS    -> role: client  (entry: port-forward / SOCKS5, multi-upstream)
#   * Kharej VPS  -> role: server  (exit: relay + iptables)
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

INSTALL_MODE="${INSTALL_MODE:-auto}"          # auto | binary | source
RELEASE_VERSION="${RELEASE_VERSION:-latest}"  # latest | vX.Y.Z

# --------------------------------------------------------------------------- #
# Theme / UI
# --------------------------------------------------------------------------- #
if [[ -t 1 ]]; then
  C_RST=$'\033[0m';  C_B=$'\033[1m';   C_DIM=$'\033[2m'
  C_BRAND=$'\033[38;5;171m'; C_ACC=$'\033[38;5;45m'
  C_OK=$'\033[38;5;42m';     C_WARN=$'\033[38;5;220m'
  C_ERR=$'\033[38;5;203m';   C_MUT=$'\033[38;5;245m'
else
  C_RST=""; C_B=""; C_DIM=""; C_BRAND=""; C_ACC=""; C_OK=""; C_WARN=""; C_ERR=""; C_MUT=""
fi

info() { printf '%s i %s %s\n' "${C_ACC}${C_B}" "$C_RST" "$*"; }
ok()   { printf '%s ok %s %s\n' "${C_OK}${C_B}" "$C_RST" "$*"; }
warn() { printf '%s ! %s %s\n' "${C_WARN}${C_B}" "$C_RST" "$*" >&2; }
err()  { printf '%s x %s %s\n' "${C_ERR}${C_B}" "$C_RST" "$*" >&2; exit 1; }

rule() { printf '%s%s%s\n' "$C_DIM" "------------------------------------------------------------" "$C_RST"; }

section() {
  printf '\n%s%s== %s%s\n' "$C_ACC" "$C_B" "$*" "$C_RST"
  printf '%s%s%s\n' "$C_DIM" "------------------------------------------------------------" "$C_RST"
}

kv() { printf '   %s%-22s%s %s\n' "$C_MUT" "$1" "$C_RST" "$2"; }

banner() {
  printf '%s%s\n' "$C_BRAND" "$C_B"
  cat <<'ART'
   ___                  _   ___                  _
  | _ \__ _ __ _ ___ __| |_| _ \_ _ ___ _ __ (_)_  _ _ __
  |  _/ _` / _` / -_) _|  _|  _/ '_/ -_) '  \| | || | '  \
  |_| \__,_\__, \___\__|\__|_| |_| \___|_|_|_|_|\_,_|_|_|_|
           |___/
ART
  printf '%s' "$C_RST"
  printf '   %spacket-level tunnel for Linux VPS%s  %s(%s)%s\n' "$C_ACC" "$C_RST" "$C_DIM" "$REPO_BRANCH" "$C_RST"
}

# prompt <var> <question> [default]
prompt() {
  local __var="$1" __q="$2" __def="${3:-}" __ans=""
  if [[ -n "$__def" ]]; then
    printf '%s%s%s %s[%s]%s: ' "$C_B" "$__q" "$C_RST" "$C_DIM" "$__def" "$C_RST" >/dev/tty
  else
    printf '%s%s%s: ' "$C_B" "$__q" "$C_RST" >/dev/tty
  fi
  read -r __ans </dev/tty || true
  [[ -z "$__ans" && -n "$__def" ]] && __ans="$__def"
  printf -v "$__var" '%s' "$__ans"
}

# ask_yes_no <question> [default y|n] -> exit 0 if yes
ask_yes_no() {
  local q="$1" def="${2:-n}" ans hint="[y/N]"
  [[ "$def" == y ]] && hint="[Y/n]"
  printf '%s%s%s %s%s%s ' "$C_B" "$q" "$C_RST" "$C_DIM" "$hint" "$C_RST" >/dev/tty
  read -r ans </dev/tty || true
  ans="${ans:-$def}"
  [[ "${ans,,}" == y* ]]
}

# choose <question> opt... -> prints chosen value
choose() {
  local q="$1"; shift
  local opts=("$@") i sel
  {
    printf '%s%s%s\n' "$C_B" "$q" "$C_RST"
    for i in "${!opts[@]}"; do
      printf '   %s%d%s) %s\n' "$C_ACC" "$((i + 1))" "$C_RST" "${opts[$i]}"
    done
  } >/dev/tty
  while :; do
    printf '   %s>%s ' "$C_ACC" "$C_RST" >/dev/tty
    read -r sel </dev/tty || true
    [[ -z "$sel" ]] && sel=1
    if [[ "$sel" =~ ^[0-9]+$ ]] && (( sel >= 1 && sel <= ${#opts[@]} )); then
      printf '%s' "${opts[$((sel - 1))]}"
      return 0
    fi
    printf '   %sinvalid choice%s\n' "$C_WARN" "$C_RST" >/dev/tty
  done
}

warn_if_private() {
  case "${1%%:*}" in
    10.*|192.168.*|172.1[6-9].*|172.2[0-9].*|172.3[0-1].*|127.*)
      warn "That looks like a private/LAN address. If the server is behind NAT, use its PUBLIC IP." ;;
  esac
}

# --------------------------------------------------------------------------- #
# Pre-flight
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
    apt)     apt-get update -qq; pkg_install ca-certificates curl git iptables build-essential libpcap-dev ;;
    dnf|yum) pkg_install ca-certificates curl git iptables gcc make libpcap-devel ;;
    pacman)  pkg_install ca-certificates curl git iptables base-devel libpcap ;;
    zypper)  pkg_install ca-certificates curl git iptables gcc make libpcap-devel ;;
    *)       warn "Install manually: curl git iptables a C toolchain and libpcap headers." ;;
  esac
  ok "Dependencies ready."
}

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

# --------------------------------------------------------------------------- #
# Go toolchain + source build
# --------------------------------------------------------------------------- #
version_ge() { [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" == "$2" ]]; }
go_bin() { command -v go 2>/dev/null || echo "/usr/local/go/bin/go"; }

ensure_go() {
  local go; go="$(go_bin)"
  if [[ -x "$go" ]] || need_cmd go; then
    local v; v="$("$(go_bin)" version 2>/dev/null | awk '{print $3}' | sed 's/^go//')"
    if [[ -n "$v" ]] && version_ge "$v" "$GO_MIN"; then ok "Go ${v} detected."; return; fi
    warn "Go ${v:-unknown} older than ${GO_MIN}; fetching ${GO_DL_VERSION}."
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

SRC=""
acquire_source() {
  if [[ -f "./go.mod" ]] && grep -q "paqetpremium" "./go.mod" 2>/dev/null; then
    SRC="$(pwd)"; info "Using source in current directory: ${SRC}"; return
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

download_release_binary() {
  local arch ver base asset tmp
  case "$(uname -m)" in
    x86_64|amd64)  arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) warn "No prebuilt binary for $(uname -m)."; return 1 ;;
  esac
  ver="${RELEASE_VERSION:-latest}"
  if [[ "$ver" == "latest" ]]; then base="${REPO_URL}/releases/latest/download"
  else base="${REPO_URL}/releases/download/${ver}"; fi
  asset="${BIN_NAME}-linux-${arch}"
  tmp="$(mktemp -d)"
  info "Fetching prebuilt ${asset} (${ver})..."
  if ! curl -fsSL "${base}/${asset}" -o "${tmp}/${asset}"; then rm -rf "$tmp"; return 1; fi
  if need_cmd sha256sum && curl -fsSL "${base}/${asset}.sha256" -o "${tmp}/${asset}.sha256" 2>/dev/null; then
    if ( cd "$tmp" && sha256sum -c "${asset}.sha256" >/dev/null 2>&1 ); then ok "Checksum verified."
    else warn "Checksum mismatch; discarding download."; rm -rf "$tmp"; return 1; fi
  else
    warn "Checksum unavailable; skipping verification."
  fi
  if ! install -m 0755 "${tmp}/${asset}" "${BIN_DIR}/${BIN_NAME}"; then rm -rf "$tmp"; return 1; fi
  rm -rf "$tmp"
  ok "Installed prebuilt: ${BIN_DIR}/${BIN_NAME} ($(${BIN_DIR}/${BIN_NAME} version 2>/dev/null || echo "${BIN_NAME}"))"
}

provision_binary() {
  if [[ "${INSTALL_MODE}" != "source" ]]; then
    install_runtime_deps
    if download_release_binary; then return; fi
    [[ "${INSTALL_MODE}" == "binary" ]] && err "Prebuilt binary unavailable and INSTALL_MODE=binary."
    warn "No prebuilt binary available; building from source instead."
  fi
  install_deps
  build_binary
}

# --------------------------------------------------------------------------- #
# Detection
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
detect_public_ip() {
  local ip svc
  for svc in "https://api.ipify.org" "https://ifconfig.me" "https://ip.sb" "https://ipinfo.io/ip"; do
    ip="$(curl -fsS --max-time 5 "$svc" 2>/dev/null | tr -dc '0-9.')"
    [[ "$ip" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] && { printf '%s' "$ip"; return 0; }
  done
  return 1
}
gen_secret() {
  if need_cmd openssl; then openssl rand -hex 16
  else tr -dc 'a-f0-9' </dev/urandom | head -c 32; echo; fi
}

# --------------------------------------------------------------------------- #
# Config state + writers
# --------------------------------------------------------------------------- #
CFG_NAME=""; CFG_IFACE=""; CFG_IPV4=""; CFG_IPV6=""; CFG_RMAC=""; CFG_RMAC6=""
CFG_PORT=""; CFG_KEY=""; CFG_TRANSPORT="kcp"; CFG_CONN="6"
CFG_STRATEGY="failover"; CFG_FWD_PORTS=""; CFG_SOCKS_PORT=""; CFG_SOCKS_BIND="127.0.0.1"
CFG_ADMIN="127.0.0.1:9090"; CFG_TOKEN=""
UP_NAMES=(); UP_IPS=(); UP_PORTS=(); UP_KEYS=(); UP_PRIOS=(); UP_WEIGHTS=()

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

emit_network() {
  local zero="$1"  # "yes" => port :0 (client)
  echo "network:"
  echo "  interface: ${CFG_IFACE}"
  echo "  ipv4:"
  if [[ "$zero" == "yes" ]]; then
    echo "    addr: \"${CFG_IPV4}:0\""
  else
    echo "    addr: \"${CFG_IPV4}:${CFG_PORT}\""
  fi
  echo "    router_mac: \"${CFG_RMAC}\""
  if [[ -n "${CFG_IPV6}" ]]; then
    echo "  ipv6:"
    if [[ "$zero" == "yes" ]]; then
      echo "    addr: \"[${CFG_IPV6}]:0\""
    else
      echo "    addr: \"[${CFG_IPV6}]:${CFG_PORT}\""
    fi
    echo "    router_mac: \"${CFG_RMAC6:-$CFG_RMAC}\""
  fi
  echo "  tcp:"
  echo "    local_flag: [\"PA\"]"
  if [[ "$zero" == "yes" ]]; then
    echo "    remote_flag: [\"PA\"]"
  fi
}

write_server_config() {
  local cfg="$1"; mkdir -p "$(dirname "$cfg")"
  {
    echo "core: paqetpremium"
    echo "version: 2"
    echo "role: server"
    echo "name: ${CFG_NAME:-kharej-exit}"
    echo
    echo "log:"; echo "  level: info"; echo
    echo "listen:"; echo "  addr: \":${CFG_PORT}\""; echo
    emit_network "no"; echo
    emit_transport; echo
    emit_admin
  } >"$cfg"
  chmod 0640 "$cfg"
  ok "Server config written: ${cfg}"
}

write_client_config() {
  local cfg="$1"; mkdir -p "$(dirname "$cfg")"
  {
    echo "core: paqetpremium"
    echo "version: 2"
    echo "role: client"
    echo "name: ${CFG_NAME:-iran-entry}"
    echo
    echo "log:"; echo "  level: info"; echo
    echo "upstream:"
    echo "  strategy: ${CFG_STRATEGY}"
    echo "  health_check:"
    echo "    interval: 10s"
    echo "    timeout: 3s"
    echo "    fail_threshold: 3"
    echo "    recover_threshold: 2"
    echo "  servers:"
    local i
    for i in "${!UP_IPS[@]}"; do
      echo "    - name: ${UP_NAMES[$i]}"
      echo "      addr: ${UP_IPS[$i]}:${UP_PORTS[$i]}"
      echo "      key: ${UP_KEYS[$i]}"
      echo "      priority: ${UP_PRIOS[$i]}"
      echo "      weight: ${UP_WEIGHTS[$i]}"
    done
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
    emit_network "yes"; echo
    emit_transport; echo
    emit_admin
  } >"$cfg"
  chmod 0640 "$cfg"
  ok "Client config written: ${cfg}"
}

# --------------------------------------------------------------------------- #
# systemd
# --------------------------------------------------------------------------- #
write_unit() {
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

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
}

install_service() {
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
    warn "Admin healthcheck not responding yet; check: journalctl -u <unit> -f"
  fi
}

# --------------------------------------------------------------------------- #
# Shared wizard pieces
# --------------------------------------------------------------------------- #
ask_transport() { CFG_TRANSPORT="$(choose 'Transport protocol' kcp quic)"; }

ask_common_network() {
  local d_iface d_ipv4 d_ipv6 d_mac
  d_iface="$(detect_iface)"; d_ipv4="$(detect_ipv4 "$d_iface")"
  d_ipv6="$(detect_ipv6 "$d_iface")"; d_mac="$(detect_gw_mac)"
  prompt CFG_IFACE "Network interface" "${d_iface}"
  prompt CFG_IPV4  "Local IPv4 (this host's interface IP)" "${d_ipv4}"
  prompt CFG_RMAC  "Gateway/router MAC" "${d_mac}"
  if ask_yes_no "Enable IPv6?" "$([[ -n "$d_ipv6" ]] && echo y || echo n)"; then
    prompt CFG_IPV6  "Local IPv6" "${d_ipv6}"
    prompt CFG_RMAC6 "IPv6 gateway MAC" "${d_mac}"
  else
    CFG_IPV6=""; CFG_RMAC6=""
  fi
}

add_upstream_servers() {
  UP_NAMES=(); UP_IPS=(); UP_PORTS=(); UP_KEYS=(); UP_PRIOS=(); UP_WEIGHTS=()
  CFG_STRATEGY="$(choose 'Load-balancing strategy' failover round_robin weighted least_latency)"
  local def_key; def_key="$(gen_secret)"
  prompt CFG_KEY "Default shared secret key (must match the server)" "${def_key}"
  local idx=1
  while :; do
    section "Upstream server #${idx}"
    local sname sip sport skey sprio sweight
    prompt sip "  Server PUBLIC IP" ""
    [[ -n "$sip" ]] || { warn "IP is required."; continue; }
    warn_if_private "$sip"
    prompt sport "  Server port" "${CFG_PORT:-8888}"
    prompt sname "  Server name" "kharej-${idx}"
    prompt skey  "  Secret key" "${CFG_KEY}"
    sprio="${idx}"; sweight="1"
    case "$CFG_STRATEGY" in
      failover|least_latency) prompt sprio "  Priority (lower = preferred)" "${idx}" ;;
      weighted) prompt sweight "  Weight" "1" ;;
    esac
    UP_NAMES+=("$sname"); UP_IPS+=("$sip"); UP_PORTS+=("$sport")
    UP_KEYS+=("$skey"); UP_PRIOS+=("$sprio"); UP_WEIGHTS+=("$sweight")
    CFG_PORT="${sport}"
    idx=$((idx + 1))
    ask_yes_no "Add another upstream server?" "n" || break
  done
}

ask_client_services() {
  prompt CFG_FWD_PORTS "Forward TCP ports (comma-separated, blank = none)" "443,8443"
  prompt CFG_SOCKS_PORT "SOCKS5 listen port (blank = disabled)" "1080"
  [[ -z "${CFG_FWD_PORTS}" && -z "${CFG_SOCKS_PORT}" ]] && err "Provide at least a forward port or a SOCKS5 port."
  prompt CFG_ADMIN "Admin API listen" "127.0.0.1:9090"
  prompt CFG_TOKEN "Admin API token (blank = none)" ""
}

summary_card() {
  section "Summary"
  for kvpair in "$@"; do kv "${kvpair%%=*}" "${kvpair#*=}"; done
  echo
}

# --------------------------------------------------------------------------- #
# Wizards
# --------------------------------------------------------------------------- #
wizard_server() {
  require_root; require_linux; require_systemd
  clear 2>/dev/null || true; banner
  section "Server (Kharej / exit) setup"
  provision_binary
  ask_common_network
  prompt CFG_PORT "Tunnel listen port" "8888"
  ask_transport
  local def_key; def_key="$(gen_secret)"
  prompt CFG_KEY "Shared secret key (give the SAME to clients)" "${def_key}"
  prompt CFG_ADMIN "Admin API listen" "127.0.0.1:9090"
  prompt CFG_TOKEN "Admin API token (blank = none)" ""
  CFG_NAME="kharej-exit"

  local pub; pub="$(detect_public_ip || true)"
  summary_card \
    "role=server" "interface=${CFG_IFACE}" "bind IP=${CFG_IPV4}" \
    "public IP=${pub:-$CFG_IPV4}" "port=${CFG_PORT}" "transport=${CFG_TRANSPORT}" \
    "secret=${CFG_KEY}" "admin=${CFG_ADMIN}"
  ask_yes_no "Write config and start the service?" "y" || { warn "Cancelled."; return; }

  write_server_config "${CONFIG_DIR}/server.yaml"
  "${BIN_DIR}/${BIN_NAME}" test -c "${CONFIG_DIR}/server.yaml" || warn "Config test reported issues (live checks need a peer)."
  install_service server
  systemctl restart "${BIN_NAME}-server.service" || warn "service did not start; inspect: journalctl -u ${BIN_NAME}-server -e"
  healthcheck "${CFG_ADMIN}"

  section "Server ready"
  kv "Secret key" "${C_B}${CFG_KEY}${C_RST}"
  if [[ -n "$pub" && "$pub" != "${CFG_IPV4}" ]]; then
    warn "Behind NAT: interface ${CFG_IPV4} vs public ${pub}."
    kv "Client server addr" "${C_B}${pub}:${CFG_PORT}${C_RST}  (NOT ${CFG_IPV4})"
  else
    kv "Client server addr" "${C_B}${CFG_IPV4}:${CFG_PORT}${C_RST}"
  fi
  kv "Manage" "systemctl status ${BIN_NAME}-server | journalctl -u ${BIN_NAME}-server -f"
}

wizard_client() {
  require_root; require_linux; require_systemd
  clear 2>/dev/null || true; banner
  section "Client (Iran / entry) setup - multi-upstream"
  provision_binary
  ask_common_network
  ask_transport
  add_upstream_servers
  ask_client_services
  CFG_NAME="iran-entry"

  local lines=("role=client" "interface=${CFG_IFACE}" "transport=${CFG_TRANSPORT}" "strategy=${CFG_STRATEGY}")
  local i
  for i in "${!UP_IPS[@]}"; do
    lines+=("upstream $((i + 1))=${UP_NAMES[$i]} ${UP_IPS[$i]}:${UP_PORTS[$i]} (prio ${UP_PRIOS[$i]}, w ${UP_WEIGHTS[$i]})")
  done
  lines+=("forward=${CFG_FWD_PORTS:-none}" "socks5=${CFG_SOCKS_PORT:-disabled}" "admin=${CFG_ADMIN}")
  summary_card "${lines[@]}"
  ask_yes_no "Write config and start the service?" "y" || { warn "Cancelled."; return; }

  write_client_config "${CONFIG_DIR}/client.yaml"
  "${BIN_DIR}/${BIN_NAME}" test -c "${CONFIG_DIR}/client.yaml" || warn "Config test reported issues (live checks need a reachable server)."
  install_service client
  systemctl restart "${BIN_NAME}-client.service" || warn "service did not start; inspect: journalctl -u ${BIN_NAME}-client -e"
  healthcheck "${CFG_ADMIN}"

  section "Client ready"
  kv "Upstreams" "${#UP_IPS[@]} server(s), strategy ${CFG_STRATEGY}"
  kv "Manage" "systemctl status ${BIN_NAME}-client | journalctl -u ${BIN_NAME}-client -f"
}

wizard_add_tunnel() {
  require_root; require_linux; require_systemd
  clear 2>/dev/null || true; banner
  section "Add named client tunnel (multi-instance)"
  [[ -x "${BIN_DIR}/${BIN_NAME}" ]] || provision_binary
  install_client_template
  local name
  prompt name "Tunnel instance name (e.g. v2ray-1)" ""
  [[ -n "$name" ]] || err "Instance name is required."
  ask_common_network
  ask_transport
  add_upstream_servers
  ask_client_services
  CFG_NAME="$name"

  summary_card "instance=${name}" "transport=${CFG_TRANSPORT}" "strategy=${CFG_STRATEGY}" \
    "upstreams=${#UP_IPS[@]}" "forward=${CFG_FWD_PORTS:-none}" "socks5=${CFG_SOCKS_PORT:-disabled}"
  ask_yes_no "Write config and start instance '${name}'?" "y" || { warn "Cancelled."; return; }

  write_client_config "${TUNNELS_DIR}/${name}.yaml"
  systemctl enable "${BIN_NAME}-client@${name}.service" >/dev/null 2>&1 || true
  systemctl restart "${BIN_NAME}-client@${name}.service" || warn "instance did not start; inspect: journalctl -u ${BIN_NAME}-client@${name} -e"
  ok "Tunnel '${name}' started (${BIN_NAME}-client@${name})."
}

# --------------------------------------------------------------------------- #
# Management
# --------------------------------------------------------------------------- #
list_units() {
  section "${APP_NAME} services"
  local found=0 u st
  for u in "${BIN_NAME}-server.service" "${BIN_NAME}-client.service"; do
    if systemctl list-unit-files --no-legend "$u" 2>/dev/null | grep -q .; then
      st="$(systemctl is-active "$u" 2>/dev/null || true)"
      kv "$u" "$st"; found=1
    fi
  done
  if [[ -d "${TUNNELS_DIR}" ]]; then
    local f base
    for f in "${TUNNELS_DIR}"/*.yaml; do
      [[ -e "$f" ]] || continue
      base="$(basename "$f" .yaml)"
      st="$(systemctl is-active "${BIN_NAME}-client@${base}.service" 2>/dev/null || true)"
      kv "${BIN_NAME}-client@${base}" "$st"; found=1
    done
  fi
  [[ "$found" -eq 1 ]] || info "No services installed yet."
}

cmd_status() {
  list_units
  if [[ -x "${BIN_DIR}/${BIN_NAME}" ]] && need_cmd curl; then
    local listen
    listen="$(awk '/^admin:/{a=1} a&&/listen:/{gsub(/[ "\047]/,"",$2);print $2;exit}' "${CONFIG_DIR}/server.yaml" "${CONFIG_DIR}/client.yaml" 2>/dev/null | head -1)"
    [[ -n "${listen:-}" ]] && { echo; curl -fsS --max-time 3 "http://${listen}/api/v1/status" 2>/dev/null || true; }
  fi
}

resolve_unit() {
  local arg="${1:-}"
  if [[ -z "$arg" ]]; then
    if systemctl list-unit-files --no-legend "${BIN_NAME}-server.service" 2>/dev/null | grep -q .; then
      echo "${BIN_NAME}-server.service"
    else echo "${BIN_NAME}-client.service"; fi
  elif [[ "$arg" == "server" || "$arg" == "client" ]]; then echo "${BIN_NAME}-${arg}.service"
  else echo "${BIN_NAME}-client@${arg}.service"; fi
}

cmd_logs()    { require_systemd; journalctl -u "$(resolve_unit "${1:-}")" -f --no-pager; }
cmd_restart() { require_root; require_systemd; local u; u="$(resolve_unit "${1:-}")"; systemctl restart "$u" && ok "Restarted ${u}"; }
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
  ask_yes_no "Proceed?" "n" || { info "Cancelled."; return; }
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
  ask_yes_no "Also delete ${CONFIG_DIR}?" "n" && { rm -rf "${CONFIG_DIR}"; ok "Removed ${CONFIG_DIR}"; }
}

# --------------------------------------------------------------------------- #
# Menu + dispatch
# --------------------------------------------------------------------------- #
menu() {
  clear 2>/dev/null || true
  banner
  printf '\n'
  printf '   %s1%s) Install / configure  %sserver%s  (Kharej / exit)\n'  "$C_ACC" "$C_RST" "$C_DIM" "$C_RST"
  printf '   %s2%s) Install / configure  %sclient%s  (Iran / entry, multi-upstream)\n' "$C_ACC" "$C_RST" "$C_DIM" "$C_RST"
  printf '   %s3%s) Add named client tunnel (multi-instance)\n' "$C_ACC" "$C_RST"
  printf '   %s4%s) Status\n'                 "$C_ACC" "$C_RST"
  printf '   %s5%s) Follow logs\n'            "$C_ACC" "$C_RST"
  printf '   %s6%s) Reload config (SIGHUP)\n' "$C_ACC" "$C_RST"
  printf '   %s7%s) Restart service\n'        "$C_ACC" "$C_RST"
  printf '   %s8%s) Update (from repo/release)\n' "$C_ACC" "$C_RST"
  printf '   %s9%s) Uninstall\n'              "$C_ACC" "$C_RST"
  printf '   %s0%s) Exit\n\n'                 "$C_ACC" "$C_RST"
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

  (none)        Interactive menu
  server        Guided server (Kharej) setup
  client        Guided client (Iran) setup - multi-upstream
  add-tunnel    Add a named client instance
  status|list   Show services / admin status
  logs   [unit] Follow logs (server|client|<tunnel>)
  reload [unit] Reload config via SIGHUP
  restart[unit] Restart a service
  update        Rebuild/refresh binary and restart services
  uninstall     Remove services and binary
  help          Show this help

Environment:
  INSTALL_MODE     auto (default) | binary | source
  RELEASE_VERSION  latest (default) | vX.Y.Z

Repository: ${REPO_URL} (branch: ${REPO_BRANCH})
EOF
}

main() {
  local cmd="${1:-menu}"; shift || true
  case "$cmd" in
    menu)       require_root; require_linux; require_systemd
                while true; do menu; printf '\n'; read -rp "Press Enter to continue..." </dev/tty || true; done ;;
    server)     wizard_server ;;
    client)     wizard_client ;;
    add-tunnel) wizard_add_tunnel ;;
    status)     cmd_status ;;
    list)       list_units ;;
    logs)       cmd_logs "${1:-}" ;;
    reload)     cmd_reload "${1:-}" ;;
    restart)    cmd_restart "${1:-}" ;;
    update)     cmd_update ;;
    uninstall)  cmd_uninstall ;;
    help|-h|--help) usage ;;
    *)          usage; err "Unknown command: ${cmd}" ;;
  esac
}

main "$@"
