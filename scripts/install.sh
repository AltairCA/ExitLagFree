#!/usr/bin/env bash
# ExitLagFree node installer for Debian/Ubuntu (also works on most systemd distros).
#
#   curl -fsSL https://github.com/AltairCA/ExitLagFree/releases/latest/download/install.sh | sudo bash
#
# Options (pass after `bash -s --` when piping):
#   --public-host HOST   IP or domain clients connect to (auto-detected if omitted)
#   --name NAME          node display name (default: hostname)
#   --version TAG        release tag to install (default: latest)
#   --binary PATH        install a local exitlag-node binary instead of downloading
#   --api-port N --wg-port N --probe-port N --max-devices N
#   --egress-allowlist CIDR,CIDR   only let peers reach these ranges
set -euo pipefail

REPO="${EXITLAGFREE_REPO:-AltairCA/ExitLagFree}"
VERSION="latest"
PUBLIC_HOST=""
NODE_NAME="$(hostname)"
BINARY=""
API_PORT=8443
WG_PORT=51820
PROBE_PORT=51821
MAX_DEVICES=10
ALLOWLIST=""
CONFIG=/etc/exitlag-node/config.json
BIN_PATH=/usr/local/bin/exitlag-node
UNIT=/etc/systemd/system/exitlag-node.service

log()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --public-host) PUBLIC_HOST="$2"; shift 2 ;;
    --name) NODE_NAME="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --binary) BINARY="$2"; shift 2 ;;
    --api-port) API_PORT="$2"; shift 2 ;;
    --wg-port) WG_PORT="$2"; shift 2 ;;
    --probe-port) PROBE_PORT="$2"; shift 2 ;;
    --max-devices) MAX_DEVICES="$2"; shift 2 ;;
    --egress-allowlist) ALLOWLIST="$2"; shift 2 ;;
    -h|--help) echo "see the header of install.sh for options"; exit 0 ;;
    *) die "unknown option $1" ;;
  esac
done

[[ $EUID -eq 0 ]] || die "run as root (sudo)"
[[ "$(uname -s)" == "Linux" ]] || die "the node runs on Linux only"
command -v systemctl >/dev/null || die "systemd is required"

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported architecture $(uname -m)" ;;
esac

install_packages() {
  log "installing dependencies"
  if command -v apt-get >/dev/null; then
    DEBIAN_FRONTEND=noninteractive apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq curl ca-certificates nftables iproute2 >/dev/null
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq wireguard-tools >/dev/null 2>&1 || true
  elif command -v dnf >/dev/null; then
    dnf install -y -q curl ca-certificates nftables iproute wireguard-tools || dnf install -y -q curl nftables iproute
  elif command -v yum >/dev/null; then
    yum install -y -q curl ca-certificates nftables iproute
  else
    warn "unknown package manager; make sure curl and nftables (or iptables) are installed"
  fi
}

install_binary() {
  if [[ -n "$BINARY" ]]; then
    log "installing local binary $BINARY"
    install -m 0755 "$BINARY" "$BIN_PATH"
    return
  fi
  local base asset tmp
  if [[ "$VERSION" == "latest" ]]; then
    base="https://github.com/${REPO}/releases/latest/download"
  else
    base="https://github.com/${REPO}/releases/download/${VERSION}"
  fi
  asset="exitlag-node-linux-${ARCH}"
  tmp="$(mktemp -d)"
  log "downloading ${asset} (${VERSION})"
  curl -fsSL -o "$tmp/$asset" "$base/$asset" || { rm -rf "$tmp"; die "download failed: $base/$asset"; }
  if curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"; then
    (cd "$tmp" && grep " ${asset}\$" checksums.txt | sha256sum -c --status) || { rm -rf "$tmp"; die "checksum verification failed"; }
  else
    warn "no checksums.txt in release; skipping verification"
  fi
  install -m 0755 "$tmp/$asset" "$BIN_PATH"
  rm -rf "$tmp"
}

detect_public_host() {
  [[ -n "$PUBLIC_HOST" ]] && return
  for url in https://api.ipify.org https://ifconfig.me/ip https://icanhazip.com; do
    PUBLIC_HOST="$(curl -4 -fsS --max-time 5 "$url" 2>/dev/null | tr -d '[:space:]')" && [[ -n "$PUBLIC_HOST" ]] && break
  done
  [[ -n "$PUBLIC_HOST" ]] || die "could not detect public IP; pass --public-host"
  log "detected public IP ${PUBLIC_HOST}"
}

write_config() {
  if [[ -f "$CONFIG" ]]; then
    log "keeping existing config ${CONFIG}"
    return
  fi
  local args=(init --config "$CONFIG" --public-host "$PUBLIC_HOST" --name "$NODE_NAME"
    --api-port "$API_PORT" --wg-port "$WG_PORT" --probe-port "$PROBE_PORT" --max-devices "$MAX_DEVICES")
  [[ -n "$ALLOWLIST" ]] && args+=(--egress-allowlist "$ALLOWLIST")
  "$BIN_PATH" "${args[@]}"
}

setup_kernel() {
  if modprobe wireguard 2>/dev/null; then
    echo wireguard > /etc/modules-load.d/exitlag-node.conf
  else
    warn "kernel WireGuard module unavailable; the node will use userspace wireguard-go"
  fi
  echo "net.ipv4.ip_forward = 1" > /etc/sysctl.d/99-exitlag-node.conf
  sysctl -q -p /etc/sysctl.d/99-exitlag-node.conf
}

open_firewall() {
  if command -v ufw >/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
    log "opening ports in ufw"
    ufw allow "${API_PORT}/tcp" comment 'exitlag-node api' >/dev/null
    ufw allow "${WG_PORT}/udp" comment 'exitlag-node wireguard' >/dev/null
    ufw allow "${PROBE_PORT}/udp" comment 'exitlag-node probe' >/dev/null
  elif command -v firewall-cmd >/dev/null && firewall-cmd --state >/dev/null 2>&1; then
    log "opening ports in firewalld"
    firewall-cmd -q --permanent --add-port="${API_PORT}/tcp" --add-port="${WG_PORT}/udp" --add-port="${PROBE_PORT}/udp"
    # firewalld's own forward chain rejects traffic from interfaces outside a
    # forwarding zone; the node's input chain still blocks access to the host.
    firewall-cmd -q --permanent --zone=trusted --add-interface=wg-elf
    firewall-cmd -q --reload
  fi
}

write_unit() {
  log "installing systemd unit"
  cat > "$UNIT" <<'EOF'
[Unit]
Description=ExitLagFree relay node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/exitlag-node serve --config /etc/exitlag-node/config.json
Restart=on-failure
RestartSec=3
RuntimeDirectory=exitlag-node
RuntimeDirectoryMode=0700
StateDirectory=exitlag-node
StateDirectoryMode=0700
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable exitlag-node >/dev/null 2>&1
  systemctl restart exitlag-node
}

wait_ready() {
  for _ in $(seq 1 30); do
    [[ -S /run/exitlag-node/admin.sock ]] && return 0
    sleep 0.5
  done
  journalctl -u exitlag-node -n 30 --no-pager >&2 || true
  die "exitlag-node did not start; see logs above"
}

install_packages
install_binary
detect_public_host
write_config
setup_kernel
open_firewall
write_unit
wait_ready

log "exitlag-node is running"
"$BIN_PATH" status
echo
"$BIN_PATH" invite --name "first-device" --ttl 1h --qr
cat <<EOF

Next steps:
  * If your provider has a cloud firewall / security group, allow inbound
    TCP ${API_PORT}, UDP ${WG_PORT} and UDP ${PROBE_PORT}.
  * Create more invites:   sudo exitlag-node invite --name "friend-pc"
  * List devices:          sudo exitlag-node devices
  * Revoke a device:       sudo exitlag-node devices revoke <id>
  * Docs and troubleshooting: https://altairca.github.io/ExitLagFree
EOF
