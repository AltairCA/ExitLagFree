# Servers with existing services

The node is designed to sit next to whatever else your VPS runs: web servers, Docker containers, other VPNs. This page lists what it touches, what can conflict, and how to fix each case.

## What the node adds

When `exitlag-node` starts, it:

1. Creates a WireGuard interface named `wg-elf` (configurable via `interface` in the [config file](/reference/node-config)). An existing interface with the same name is deleted first.
2. Listens on TCP `api_port` (8443), UDP `wg_port` (51820) and UDP `probe_port` (51821).
3. Installs its firewall rules:
   - **nftables** (preferred): one table, `inet exitlagfree`, with `input` and `forward` filter chains at priority `-5` and a `postrouting` NAT chain. They only match packets entering or leaving `wg-elf`, plus masquerade for the tunnel subnet.
   - **iptables** (if `nft` isn't installed): chains `ELF-IN`, `ELF-FWD`, optionally `ELF-EGRESS`, and `ELF-POST` in the nat table, each hooked in with one jump rule.
   - In both cases, two `FORWARD` accept rules in iptables for traffic to and from `wg-elf`. They let tunnel traffic through hosts whose iptables FORWARD policy is DROP, such as ufw and Docker hosts. Everything they accept has already been filtered by the node's own chain.
4. Writes `1` to `/proc/sys/net/ipv4/ip_forward`.

On shutdown it deletes its table or chains and the two accept rules. It never flushes, reorders or rewrites anything else.

## Port conflicts

```bash
ss -lntup | grep -E ':(8443|51820|51821)\b'
```

Common clashes:
- **UDP 51820**: another WireGuard setup (wg-quick, wg-easy, PiVPN, Netmaker). Use `--wg-port 51830` or similar.
- **TCP 8443**: control panels and alternative HTTPS ports (Plesk, UniFi, some Docker apps). Use `--api-port 9443` or similar.

Ports are set with `exitlag-node init` or by editing `/etc/exitlag-node/config.json` and restarting the service. Remember to open the new ports in your firewall and cloud firewall.

::: warning Changing ports after pairing
Devices remember the node's ports when they pair. After changing `wg_port`, `probe_port` or `api_port`, remove the node in the app and pair it again with a new invite.
:::

## Subnet overlaps

The tunnel uses `10.66.0.0/24` by default; the node takes `10.66.0.1` and gives each device one address. It must not overlap any network on the VPS:

```bash
ip -4 addr | grep inet
ip route
docker network inspect $(docker network ls -q) 2>/dev/null | grep '"Subnet"'
```

Pick an unused private range such as `10.77.0.0/24` with `--subnet` (or `subnet` in the config). Like ports, changing the subnet after pairing requires re-pairing devices.

## Host firewalls

### ufw

Allow the three ports (the installer does this automatically when ufw is active):

```bash
ufw allow 8443/tcp && ufw allow 51820/udp && ufw allow 51821/udp
```

No other ufw changes are needed. ufw's default `DROP` forward policy is handled by the node's two FORWARD accept rules. With the default `MANAGE_BUILTINS=no` in `/etc/default/ufw`, `ufw reload` leaves those rules alone. If you set `MANAGE_BUILTINS=yes`, run `systemctl restart exitlag-node` after reloading ufw.

### firewalld

Open the ports, and put the tunnel interface in the `trusted` zone so firewalld forwards its traffic:

```bash
firewall-cmd --permanent --add-port=8443/tcp --add-port=51820/udp --add-port=51821/udp
firewall-cmd --permanent --zone=trusted --add-interface=wg-elf
firewall-cmd --reload
```

The `trusted` zone doesn't open the VPS to tunnel devices: the node's own input chain drops everything from `wg-elf` to the host except ping to the gateway, and that drop applies regardless of firewalld. firewalld reloads only rebuild firewalld's own table, so the node's rules survive them.

### Your own nftables ruleset

Two things to watch for in `/etc/nftables.conf`:

**1. `flush ruleset` at the top.** Many distributions ship this line. Running `systemctl restart nftables` (or `nft -f /etc/nftables.conf`) then deletes every table, including `inet exitlagfree`, and tunnel traffic stops. Either:
- restart the node after reloading your rules: `systemctl restart exitlag-node`; or
- replace `flush ruleset` with a delete of only your own tables, so other tables survive:

```
table inet filter
delete table inet filter
table inet filter {
  # ... your rules ...
}
```

Also order the node after nftables at boot: see [step 7 of the manual install](/node/manual-install#_7-install-the-systemd-service).

**2. A forward chain with `policy drop`.** In nftables, `accept` in one base chain doesn't stop other base chains on the same hook from dropping the packet. If your ruleset has its own forward chain that drops by default, allow tunnel traffic in it:

```
chain forward {
  type filter hook forward priority 0; policy drop;
  iifname "wg-elf" accept
  oifname "wg-elf" ct state established,related accept
  # ... your rules ...
}
```

This is safe because the node's own forward chain (priority `-5`) runs first and has already dropped anything disallowed. If your input chain drops by default, allow the three ports there too (see [step 6 of the manual install](/node/manual-install#_6-open-the-ports)).

### Plain iptables

The node handles FORWARD and NAT itself. You only need to accept the three ports in INPUT:

```bash
iptables -A INPUT -p tcp --dport 8443 -j ACCEPT
iptables -A INPUT -p udp -m multiport --dports 51820,51821 -j ACCEPT
```

If a tool restores your saved rules with a full flush (for example `iptables-restore` without `--noflush`, run by `netfilter-persistent`), restart the node afterwards.

## Docker hosts

Docker sets the iptables FORWARD policy to DROP and inserts its own chains. The node's two FORWARD accept rules let tunnel traffic through, and Docker's chains only match Docker bridges, so the two coexist. Restarting Docker doesn't remove the node's rules.

Check that the tunnel subnet doesn't overlap a Docker network (see above). To run the node itself in a container, see [Docker](/node/docker).

## Other VPNs on the same server

- **Another WireGuard server:** fine, as long as the port, subnet and interface name differ.
- **Tailscale or other mesh VPNs:** fine. Devices paired with your node can't reach Tailscale's `100.64.0.0/10` addresses, because the node blocks CGNAT ranges for paired devices.
- **OpenVPN:** fine, as long as its subnet doesn't overlap the tunnel subnet.

## Reverse proxies

Don't put the pairing API behind nginx, Caddy, Traefik or Cloudflare with TLS termination. The app pins the node's own self-signed certificate (its fingerprint is in the invite link), so any proxy that presents a different certificate makes pairing fail, by design.

Give the API its own port instead. It doesn't need to be 443 and it doesn't need a domain name.

## Limit what devices can reach

By default, paired devices can reach any public IPv4 address through the node. To make sure the node can only be used for games, set an egress allowlist with the ranges of the games you play:

```bash
exitlag-node init --force --public-host 203.0.113.10 \
  --egress-allowlist 155.133.224.0/19,162.254.192.0/21,185.25.180.0/22
```

or edit `egress_allowlist` in `/etc/exitlag-node/config.json` and run `systemctl restart exitlag-node`.

The ranges for each built-in game are in the [`profiles/`](https://github.com/AltairCA/ExitLagFree/tree/master/profiles) directory. Traffic to anything not on the list is dropped, so include every range you select in the app. Games that use large cloud regions (Fortnite, and Apex Legends' Google Cloud regions) are impractical to allowlist.

::: warning `init --force` rewrites the whole config
It resets every setting to the values you pass and the defaults for the rest. Your keys, devices and certificate in `/var/lib/exitlag-node` are kept, so paired devices keep working as long as the ports and subnet stay the same.
:::

## Quick checklist

Before you pair your first device, check that:

- the ports are free, or changed with `--api-port`, `--wg-port` and `--probe-port`;
- the tunnel subnet doesn't overlap anything on the VPS;
- the ports are open in the host firewall **and** the cloud firewall;
- with firewalld, `wg-elf` is in the `trusted` zone;
- with your own nftables ruleset, there's no `flush ruleset` (or the node restarts after reloads) and your forward chain allows `wg-elf`;
- the API isn't behind a TLS-terminating proxy.
