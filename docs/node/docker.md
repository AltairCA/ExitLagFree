# Docker

Run the node as a container if your server is already managed with Docker. The image is published for `linux/amd64` and `linux/arm64`:

```
ghcr.io/altairca/exitlagfree-node
```

| Tag | What it is |
|-----|------------|
| `latest` | Latest stable release |
| `v0.2.0` etc. | A specific release |
| `nightly` | Built from every push to `master`; may be unstable |

::: info Why host networking?
The node creates a WireGuard interface and installs NAT and filtering rules, which must live in the host's network stack for traffic to reach the internet. So the container runs with `network_mode: host` and the `NET_ADMIN` capability. It has its own filesystem, but its network changes are the same as with the [manual install](/node/manual-install): one interface, one nftables table and two iptables FORWARD rules, all removed when the container stops.
:::

## Prepare the host

These steps run on the host, not in the container.

**1. Enable IP forwarding.** The container can't change kernel settings, so the node expects forwarding to be on already. Docker usually turns it on, but make it permanent:

```bash
echo 'net.ipv4.ip_forward = 1' | sudo tee /etc/sysctl.d/99-exitlag-node.conf
sudo sysctl -p /etc/sysctl.d/99-exitlag-node.conf
```

**2. Optional: load kernel WireGuard.** Without it the node uses userspace wireguard-go (through `/dev/net/tun`), which works fine but uses a bit more CPU.

```bash
sudo modprobe wireguard && echo wireguard | sudo tee /etc/modules-load.d/exitlag-node.conf
```

**3. Open the ports** in the host firewall and your provider's cloud firewall: TCP 8443, UDP 51820, UDP 51821 by default. The per-firewall commands are in [step 6 of the manual install](/node/manual-install#_6-open-the-ports). On firewalld, also put `wg-elf` in the `trusted` zone (see [firewalld](/node/existing-services#firewalld)).

**4. Check for conflicts** with ports and subnets already in use, especially other WireGuard containers (wg-easy uses UDP 51820) and Docker networks. See [Servers with existing services](/node/existing-services).

## Run with Docker Compose

Download the example compose file:

```bash
mkdir -p ~/exitlag-node && cd ~/exitlag-node
curl -fsSLO https://raw.githubusercontent.com/AltairCA/ExitLagFree/master/deploy/docker-compose.yml
```

Edit `ELF_PUBLIC_HOST` (your VPS public IP or domain) and `ELF_NAME`, and uncomment any other settings you need:

```yaml
services:
  exitlag-node:
    image: ghcr.io/altairca/exitlagfree-node:latest
    container_name: exitlag-node
    restart: unless-stopped
    network_mode: host
    cap_add:
      - NET_ADMIN
      - NET_RAW
    devices:
      - /dev/net/tun:/dev/net/tun
    environment:
      ELF_PUBLIC_HOST: "203.0.113.10"
      ELF_NAME: "my-node"
    volumes:
      - config:/etc/exitlag-node
      - data:/var/lib/exitlag-node

volumes:
  config:
  data:
```

Start it and check the logs:

```bash
docker compose up -d
docker logs exitlag-node
```

You should see a line like `exitlag-node ... up: wg=wg-elf/kernel ... firewall=nftables`.

## Environment variables

These are only read on the **first start**, when `/etc/exitlag-node/config.json` doesn't exist yet. After that, the config file in the `config` volume is the source of truth (see [Change settings](#change-settings)).

| Variable | Default | Description |
|----------|---------|-------------|
| `ELF_PUBLIC_HOST` | (required) | Public IP or domain clients connect to |
| `ELF_NAME` | container hostname | Name shown in the app |
| `ELF_API_PORT` | `8443` | TCP port of the pairing API |
| `ELF_WG_PORT` | `51820` | UDP port for WireGuard |
| `ELF_PROBE_PORT` | `51821` | UDP port for the latency probe |
| `ELF_SUBNET` | `10.66.0.0/24` | Tunnel subnet |
| `ELF_MAX_DEVICES` | `10` | Maximum paired devices |
| `ELF_EGRESS_ALLOWLIST` | empty | Comma-separated CIDRs devices may reach |

With host networking the container's hostname is the host's, so the default name matches the VPS hostname.

## Pair devices and manage the node

Run the node's CLI inside the container:

```bash
docker exec exitlag-node exitlag-node invite --name "my-pc" --qr
docker exec exitlag-node exitlag-node devices
docker exec exitlag-node exitlag-node devices revoke <id>
docker exec exitlag-node exitlag-node status
```

Everything in [Managing your node](/node/managing) applies, with this prefix.

## Change settings

Edit the config file in the volume, then restart:

```bash
docker run --rm -it -v exitlag-node_config:/cfg alpine vi /cfg/config.json
docker compose restart
```

The volume name is `<project>_config`; check it with `docker volume ls`. All fields are described in [Node config file](/reference/node-config).

## Updating

```bash
docker compose pull
docker compose up -d
```

Keys, devices and the certificate live in the `data` volume, so paired devices keep working. Pin a release tag (for example `ghcr.io/altairca/exitlagfree-node:v0.2.0`) instead of `latest` if you prefer to update deliberately.

## Without Compose

```bash
docker run -d --name exitlag-node --restart unless-stopped \
  --network host --cap-add NET_ADMIN --cap-add NET_RAW --device /dev/net/tun \
  -e ELF_PUBLIC_HOST=203.0.113.10 -e ELF_NAME=my-node \
  -v exitlag-node-config:/etc/exitlag-node -v exitlag-node-data:/var/lib/exitlag-node \
  ghcr.io/altairca/exitlagfree-node:latest
```

## Notes

- **iptables backend:** the image uses the nftables-based `iptables`, the default on current Debian, Ubuntu, Fedora and RHEL. If your host still uses `iptables-legacy` (`iptables -V` on the host shows `legacy`) together with ufw or Docker's DROP forward policy, add the two accept rules on the host yourself:
  ```bash
  iptables -I FORWARD 1 -i wg-elf -j ACCEPT
  iptables -I FORWARD 1 -o wg-elf -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  ```
- **Back up** the `config` and `data` volumes to move the node to another server.
- **Uninstall:** `docker compose down` removes the container and its network changes; `docker compose down -v` also deletes the volumes, which unpairs every device.
