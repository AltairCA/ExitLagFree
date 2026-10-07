# Manual install

Use this when your VPS already hosts other services and you want to control every change, or when the default ports or subnet are taken. You end up with the same setup as the [one-command install](/node/install-script).

All commands run as root (`sudo -i`).

## 1. Pick ports and a subnet

The node needs one TCP port and two UDP ports, plus a private subnet for tunnel addresses:

| Setting | Default | Change with |
|---------|---------|-------------|
| Pairing API | TCP 8443 | `--api-port` |
| WireGuard | UDP 51820 | `--wg-port` |
| Latency probe | UDP 51821 | `--probe-port` |
| Tunnel subnet | `10.66.0.0/24` | `--subnet` |
| Interface name | `wg-elf` | `interface` in the config file |

Check what's already in use:

```bash
# listening ports
ss -lntup | grep -E ':(8443|51820|51821)\b'

# existing WireGuard interfaces and their ports (e.g. wg-easy, a personal VPN)
wg show 2>/dev/null | grep -E 'interface|listening port'

# routes and Docker networks that might overlap 10.66.0.0/24
ip route | grep -E '^10\.66\.'
docker network inspect $(docker network ls -q) 2>/dev/null | grep -E '"Subnet"'
```

If any of these show a conflict, pick other values, for example `--wg-port 51830 --probe-port 51831 --subnet 10.77.0.0/24`. The subnet must be IPv4 and `/29` or larger. A `/24` allows 253 devices.

::: tip Choose before pairing
Devices remember the node's ports and their tunnel address when they pair. If you change `wg_port`, `probe_port`, `api_port` or `subnet` later, already-paired devices have to be removed and paired again.
:::

## 2. Install dependencies

The node uses `nft` (or `iptables`) and the `ip` tool. WireGuard tools are optional but handy for debugging.

::: code-group

```bash [Debian / Ubuntu]
apt-get update
apt-get install -y curl ca-certificates nftables iproute2 wireguard-tools
```

```bash [Fedora / RHEL / Rocky]
dnf install -y curl ca-certificates nftables iproute wireguard-tools
```

:::

If your server is set up around iptables (not nftables), you can skip `nftables`. The node falls back to iptables when `nft` isn't installed.

## 3. Download and verify the binary

```bash
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
BASE=https://github.com/AltairCA/ExitLagFree/releases/latest/download
cd /tmp
curl -fLO "$BASE/exitlag-node-linux-$ARCH"
curl -fLO "$BASE/checksums.txt"
grep " exitlag-node-linux-$ARCH\$" checksums.txt | sha256sum -c -
install -m 0755 "exitlag-node-linux-$ARCH" /usr/local/bin/exitlag-node
exitlag-node version
```

For a specific release, replace `latest/download` with `download/v0.2.0` (or `download/nightly`).

## 4. Write the config

```bash
exitlag-node init \
  --public-host 203.0.113.10 \
  --name "Sydney" \
  --api-port 8443 \
  --wg-port 51820 \
  --probe-port 51821 \
  --subnet 10.66.0.0/24 \
  --max-devices 10
```

- `--public-host` is the IP or domain the app will connect to. Find your public IP with `curl -4 https://api.ipify.org`.
- Add `--egress-allowlist 155.133.224.0/19,...` to restrict what devices can reach (see [Limit what devices can reach](/node/existing-services#limit-what-devices-can-reach)).

This writes `/etc/exitlag-node/config.json` with mode `0600`. All fields are described in [Node config file](/reference/node-config).

## 5. Enable WireGuard and IP forwarding

```bash
# Kernel WireGuard (optional; without it the node uses userspace wireguard-go)
modprobe wireguard && echo wireguard > /etc/modules-load.d/exitlag-node.conf

# Forward IPv4 packets between the tunnel and the internet
echo 'net.ipv4.ip_forward = 1' > /etc/sysctl.d/99-exitlag-node.conf
sysctl -p /etc/sysctl.d/99-exitlag-node.conf
```

::: info Forwarding on a shared server
Enabling `ip_forward` lets the kernel route packets between interfaces. Docker and most VPN software need it too, so it's usually on already. On its own it doesn't expose anything: the node's firewall rules decide what tunnel traffic may be forwarded, and your existing INPUT rules still protect the host.
:::

## 6. Open the ports

Open the three ports in the host firewall you use. Replace the numbers if you changed them.

::: code-group

```bash [ufw]
ufw allow 8443/tcp comment 'exitlag-node api'
ufw allow 51820/udp comment 'exitlag-node wireguard'
ufw allow 51821/udp comment 'exitlag-node probe'
```

```bash [firewalld]
firewall-cmd --permanent --add-port=8443/tcp --add-port=51820/udp --add-port=51821/udp
# let firewalld forward tunnel traffic (the node still blocks access to the host)
firewall-cmd --permanent --zone=trusted --add-interface=wg-elf
firewall-cmd --reload
```

```bash [nftables]
# Add to the input chain of your own table in /etc/nftables.conf, e.g. "table inet filter":
#   tcp dport 8443 accept
#   udp dport { 51820, 51821 } accept
nft add rule inet filter input tcp dport 8443 accept
nft add rule inet filter input udp dport '{ 51820, 51821 }' accept
```

```bash [iptables]
iptables -A INPUT -p tcp --dport 8443 -j ACCEPT
iptables -A INPUT -p udp -m multiport --dports 51820,51821 -j ACCEPT
# persist with your distro's tool, e.g. netfilter-persistent save
```

:::

Then allow the same ports in your provider's **cloud firewall or security group**, if it has one.

You don't need any forwarding or NAT rules: the node installs and removes those itself. The exception is a hand-written nftables ruleset whose forward chain drops by default; see [Your own nftables ruleset](/node/existing-services#your-own-nftables-ruleset).

## 7. Install the systemd service

This is the same hardened unit the installer uses:

```bash
cat > /etc/systemd/system/exitlag-node.service <<'EOF'
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
systemctl enable --now exitlag-node
```

If your firewall service rebuilds its rules from scratch at boot, start the node after it so its rules aren't wiped. For example, for nftables:

```bash
mkdir -p /etc/systemd/system/exitlag-node.service.d
printf '[Unit]\nAfter=nftables.service\n' > /etc/systemd/system/exitlag-node.service.d/order.conf
systemctl daemon-reload
```

## 8. Verify and pair

```bash
exitlag-node status
journalctl -u exitlag-node -n 20 --no-pager
```

`status` shows the WireGuard backend (`kernel` or `userspace`), the firewall backend (`nftables` or `iptables`) and the certificate fingerprint.

Create a pairing link and paste it into the app under **+ Add node**:

```bash
exitlag-node invite --name "my-pc" --qr
```

Continue with [Managing your node](/node/managing).
