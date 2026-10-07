# Troubleshooting

## The node doesn't start

```bash
journalctl -u exitlag-node -n 50 --no-pager      # Docker: docker logs exitlag-node
```

| Log message | Fix |
|-------------|-----|
| `config ... not found` | Run `exitlag-node init --public-host <ip>` (see [Manual install](/node/manual-install#_4-write-the-config)). |
| `public_host must be set` | Add `public_host` to the config file. |
| `api: listen tcp :8443: bind: address already in use` | Another program uses the port. Pick another `api_port` (see [Port conflicts](/node/existing-services#port-conflicts)). |
| `probe listener: ... address already in use` | Same, for `probe_port`. |
| `wireguard: configure ...: address already in use` | Another WireGuard interface uses `wg_port`. |
| `firewall: neither nft nor iptables is installed` | Install `nftables` (or `iptables`). |
| `firewall: enable ip_forward: ...` | In Docker, set `net.ipv4.ip_forward=1` on the host (see [Docker](/node/docker#prepare-the-host)). |
| `serve must run as root` | Run it through systemd or with `sudo`. |

## The app can't pair

- **"invalid or expired invite":** invites work once and expire after 15 minutes by default. Create a new one with `sudo exitlag-node invite`.
- **Timeouts:** the app can't reach the API port. Check:
  1. the service is running: `systemctl status exitlag-node`;
  2. the host firewall allows the TCP port (ufw, firewalld, nftables, iptables);
  3. your provider's **cloud firewall or security group** allows it. This is the most common cause;
  4. `public_host` is the right IP or domain. Test from your PC: `curl -k https://<public_host>:8443/v1/me` should answer quickly with an authorization error, not hang.
- **Certificate or fingerprint errors:** the API is behind something that presents a different certificate, such as a reverse proxy or a TLS-inspecting firewall. Expose the API port directly (see [Reverse proxies](/node/existing-services#reverse-proxies)).
- **"too many failed attempts, try again later":** after 10 failed attempts the IP is locked out for 15 minutes. Wait, then use a fresh invite.
- **"device limit reached on this node"** (or `invite` saying the node already has N/N devices): revoke a device or raise `max_devices`.
- **"tunnel subnet has no free addresses":** use a larger `subnet` (all devices then need to re-pair).

## Connected, but games don't go through the node

- **Handshake missing:** on the VPS, `sudo exitlag-node devices` should show a recent "last handshake" for your device. If it says `never`, UDP `wg_port` is blocked by a firewall or cloud firewall.
- **Handshake OK, but no traffic:** forwarding is blocked on the VPS.
  - firewalld: put `wg-elf` in the `trusted` zone (see [firewalld](/node/existing-services#firewalld)).
  - Your own nftables ruleset with a forward chain that drops by default: allow `wg-elf` (see [Your own nftables ruleset](/node/existing-services#your-own-nftables-ruleset)).
  - Your firewall reloaded and flushed everything: `sudo systemctl restart exitlag-node`.
  - An `egress_allowlist` that doesn't include the game's ranges.
- **Wrong ranges:** the game's servers aren't in the selected profile. Find the server IP and add it as a [custom route](/app/games#custom-routes).
- **After changing the selection:** disconnect and connect again.

## Ping through the node is worse

A relay only helps when your ISP's route to the game is worse than the VPS's route. Use **Direct vs. via node** with a server IP: if "via node" is higher, try a VPS in a different city or from another provider, or play direct for that game.

## Helper problems

- **"Install helper" does nothing or fails:** the elevation prompt was cancelled or denied. Click again and approve it.
- **Windows: "wintun.dll" errors:** you're running the portable build from inside the zip. Extract it first, or use the setup.exe.
- **"Update helper" keeps coming back:** the update prompt wasn't approved, or the helper failed to restart. Approve it again; on Windows, check that no antivirus blocked `C:\Program Files\ExitLagFree\Helper\exitlag-helper.exe`.
- **Linux AppImage:** pkexec (polkit) must be installed for the elevation prompt.

## Websites or other apps act strangely while connected

Only the selected ranges go through the node. Cloud-region profiles (Fortnite, Apex Legends' Google Cloud regions) cover whole provider regions, so some websites hosted there also go through the node. Pick fewer regions, or disconnect when you're not playing.
