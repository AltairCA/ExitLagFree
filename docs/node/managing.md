# Managing your node

All commands run on the VPS as root. With [Docker](/node/docker), prefix them with `docker exec exitlag-node`, for example `docker exec exitlag-node exitlag-node devices`.

## Invite a device

```bash
sudo exitlag-node invite --name "laptop"            # valid for 15 minutes
sudo exitlag-node invite --name "friend-pc" --ttl 2h --qr
```

Each link works **once** and expires after `--ttl` (at most 7 days). Share it privately; anyone who has an unused link can pair. Invites can't be created when the node already has `max_devices` devices.

## List and revoke devices

```bash
sudo exitlag-node devices
```

```
ID                NAME    ADDRESS    LAST HANDSHAKE  RX       TX       ENDPOINT
3f9a1c0b7d2e4a65  laptop  10.66.0.2  12s ago         4.1 MiB  1.2 MiB  198.51.100.7:53211
```

```bash
sudo exitlag-node devices revoke 3f9a1c0b7d2e4a65
```

Revoking removes the device's WireGuard key immediately; it can't connect, ping or re-pair without a new invite. A user can also remove the node from inside the app, which unpairs that device.

## Status and logs

```bash
sudo exitlag-node status
journalctl -u exitlag-node -f            # Docker: docker logs -f exitlag-node
```

`status` shows the node name and version, public host, WireGuard backend (`kernel` or `userspace`), firewall backend, device count, pending invites and the certificate fingerprint.

## Change settings

Edit `/etc/exitlag-node/config.json` (see [Node config file](/reference/node-config)), then:

```bash
sudo systemctl restart exitlag-node
```

Changing the name, `max_devices` or `egress_allowlist` is safe. Changing ports, the subnet or `public_host` means paired devices have to be removed in the app and paired again.

## Update

If you used the installer, re-run it; it keeps your config and devices:

```bash
curl -fsSL https://github.com/AltairCA/ExitLagFree/releases/latest/download/install.sh | sudo bash
```

For a manual install, repeat [step 3 of the manual install](/node/manual-install#_3-download-and-verify-the-binary), then `sudo systemctl restart exitlag-node`. For Docker, see [Updating the container](/node/docker#updating).

## Back up

Everything that identifies your node lives in two places:
- `/etc/exitlag-node/config.json`
- `/var/lib/exitlag-node/` (WireGuard key, paired devices, TLS certificate)

Restoring both on a new server with the same public IP or domain keeps all devices paired. If the IP changes, update `public_host` and re-pair.

## Uninstall

```bash
sudo systemctl disable --now exitlag-node   # also removes the node's firewall rules
sudo rm -f /etc/systemd/system/exitlag-node.service /usr/local/bin/exitlag-node
sudo rm -rf /etc/systemd/system/exitlag-node.service.d /etc/exitlag-node /var/lib/exitlag-node
sudo rm -f /etc/modules-load.d/exitlag-node.conf
sudo systemctl daemon-reload
```

Then, if you added them:

```bash
# ufw
sudo ufw delete allow 8443/tcp; sudo ufw delete allow 51820/udp; sudo ufw delete allow 51821/udp

# firewalld
sudo firewall-cmd --permanent --remove-port=8443/tcp --remove-port=51820/udp --remove-port=51821/udp
sudo firewall-cmd --permanent --zone=trusted --remove-interface=wg-elf
sudo firewall-cmd --reload
```

Leave `/etc/sysctl.d/99-exitlag-node.conf` in place if other software on the server (Docker, another VPN) needs IP forwarding; otherwise remove it too.
