# exitlag-node CLI

```
exitlag-node <command> [options]
```

Every command accepts `--config PATH` (default `/etc/exitlag-node/config.json`). The admin commands (`invite`, `devices`, `status`) talk to the running daemon through its admin socket (default `/run/exitlag-node/admin.sock`, readable by root only), so run them with `sudo`.

## init

Writes a new config file.

```bash
exitlag-node init --public-host <ip-or-domain> [options]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--public-host` | (required) | Public IP or domain that clients use to reach this VPS |
| `--name` | hostname | Display name for this node |
| `--api-port` | `8443` | TCP port of the HTTPS pairing API |
| `--wg-port` | `51820` | UDP port for WireGuard |
| `--probe-port` | `51821` | UDP port for latency probes |
| `--subnet` | `10.66.0.0/24` | Tunnel subnet |
| `--max-devices` | `10` | Maximum paired devices |
| `--egress-allowlist` | empty | Comma-separated CIDRs that devices may reach (empty means any public address) |
| `--force` | `false` | Overwrite an existing config |

## serve

Runs the daemon in the foreground. Must run as root. The systemd unit and the Docker image both use this.

```bash
exitlag-node serve
```

## invite

Creates a one-time pairing link.

```bash
exitlag-node invite [--name NAME] [--ttl 15m] [--qr]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | `device` | Name for the device that will use the invite (max 64 characters) |
| `--ttl` | `15m` | How long the invite stays valid, e.g. `30m`, `2h` (max `168h`) |
| `--qr` | `false` | Also print the link as a QR code |

Fails if the node already has `max_devices` devices.

## devices

```bash
exitlag-node devices              # same as: devices list
exitlag-node devices revoke <id>  # alias: devices rm <id>
```

`list` shows each device's ID, name, tunnel address, last handshake, traffic and current endpoint. `revoke` removes the device immediately.

## status

```bash
exitlag-node status
```

Shows the node name and version, public host, WireGuard interface and backend (`kernel` or `userspace`), firewall backend (`nftables` or `iptables`), device count, pending invites and the TLS certificate fingerprint.

## version

```bash
exitlag-node version
```
