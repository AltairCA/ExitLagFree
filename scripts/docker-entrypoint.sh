#!/bin/sh
# Writes /etc/exitlag-node/config.json from ELF_* variables on first start,
# then runs the given exitlag-node command (default: serve).
set -eu

CONFIG=/etc/exitlag-node/config.json
[ $# -gt 0 ] || set -- serve

if [ "$1" = "serve" ] && [ ! -f "$CONFIG" ]; then
  if [ -z "${ELF_PUBLIC_HOST:-}" ]; then
    echo "error: $CONFIG does not exist; set ELF_PUBLIC_HOST (the VPS public IP or domain) to create it" >&2
    exit 1
  fi
  set -- init --config "$CONFIG" --public-host "$ELF_PUBLIC_HOST"
  [ -n "${ELF_NAME:-}" ] && set -- "$@" --name "$ELF_NAME"
  [ -n "${ELF_API_PORT:-}" ] && set -- "$@" --api-port "$ELF_API_PORT"
  [ -n "${ELF_WG_PORT:-}" ] && set -- "$@" --wg-port "$ELF_WG_PORT"
  [ -n "${ELF_PROBE_PORT:-}" ] && set -- "$@" --probe-port "$ELF_PROBE_PORT"
  [ -n "${ELF_SUBNET:-}" ] && set -- "$@" --subnet "$ELF_SUBNET"
  [ -n "${ELF_MAX_DEVICES:-}" ] && set -- "$@" --max-devices "$ELF_MAX_DEVICES"
  [ -n "${ELF_EGRESS_ALLOWLIST:-}" ] && set -- "$@" --egress-allowlist "$ELF_EGRESS_ALLOWLIST"
  exitlag-node "$@"
  set -- serve
fi

if [ "$1" = "serve" ]; then
  shift
  exec exitlag-node serve --config "$CONFIG" "$@"
fi
exec exitlag-node "$@"
