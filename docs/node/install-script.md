# One-command install

```bash
curl -fsSL https://github.com/AltairCA/ExitLagFree/releases/latest/download/install.sh | sudo bash
```

The script works on Debian and Ubuntu, and on most other systemd distributions (it also knows `dnf` and `yum`). It needs root, systemd, and an amd64 or arm64 CPU.

## Options

Pass options after `bash -s --` when piping:

```bash
curl -fsSL https://github.com/AltairCA/ExitLagFree/releases/latest/download/install.sh \
  | sudo bash -s -- --public-host vps.example.com --name "Sydney" --api-port 9443
```

| Option | Default | Description |
|--------|---------|-------------|
| `--public-host HOST` | auto-detected | IP or domain the desktop app connects to. Detected via api.ipify.org, ifconfig.me or icanhazip.com if omitted. |
| `--name NAME` | the hostname | Display name shown in the app. |
| `--version TAG` | `latest` | Release to install, e.g. `v0.2.0` or `nightly`. |
| `--binary PATH` | | Install a local `exitlag-node` binary instead of downloading. |
| `--api-port N` | `8443` | TCP port of the pairing API. |
| `--wg-port N` | `51820` | UDP port for WireGuard. |
| `--probe-port N` | `51821` | UDP port for the latency probe. |
| `--max-devices N` | `10` | Maximum number of paired devices. |
| `--egress-allowlist CIDRS` | none | Comma-separated ranges paired devices may reach. Empty means any public address. |

The port, name, device and allowlist options only apply on the first install. If `/etc/exitlag-node/config.json` already exists, the script keeps it; edit it instead (see [Node config file](/reference/node-config)).

## What it changes on your server

Useful to know if the VPS already runs other services:

| What | Where |
|------|-------|
| Packages | `curl`, `ca-certificates`, `nftables`, `iproute2`, and `wireguard-tools` if available |
| Binary | `/usr/local/bin/exitlag-node` (checksum-verified against the release's `checksums.txt`) |
| Config | `/etc/exitlag-node/config.json` (only created if missing) |
| State (keys, devices, TLS certificate) | `/var/lib/exitlag-node/` |
| Kernel module | `/etc/modules-load.d/exitlag-node.conf` loads `wireguard` at boot, if the module exists |
| IP forwarding | `/etc/sysctl.d/99-exitlag-node.conf` sets `net.ipv4.ip_forward = 1` |
| Host firewall | If **ufw** is active: allows the three ports. Else if **firewalld** is running: adds them permanently and puts `wg-elf` in the `trusted` zone so firewalld forwards tunnel traffic. Otherwise nothing. |
| Service | `/etc/systemd/system/exitlag-node.service`, enabled and started |

While the service runs, the node also:
- creates the `wg-elf` WireGuard interface (an existing interface with that name is replaced);
- installs its own nftables table, `inet exitlagfree`, or `ELF-*` iptables chains if nftables isn't available;
- adds two `FORWARD` accept rules for its own interface in iptables, so hosts whose FORWARD policy is DROP (ufw, Docker) still pass tunnel traffic.

It removes the firewall rules again when it stops. It never flushes or rewrites your existing rules. See [Servers with existing services](/node/existing-services) for the details.

::: warning Cloud firewalls
The script can't open ports in your provider's firewall (AWS security groups, Oracle Cloud security lists, Hetzner Cloud Firewall and so on). Allow TCP 8443, UDP 51820 and UDP 51821 there yourself, or whichever ports you chose.
:::

## Updating

Re-run the same command. It downloads the newer binary, keeps your config and state, and restarts the service. Paired devices keep working.

## Nightly builds

```bash
curl -fsSL https://github.com/AltairCA/ExitLagFree/releases/download/nightly/install.sh | sudo bash -s -- --version nightly
```

Nightly builds come from every push to `master` and may be unstable.
