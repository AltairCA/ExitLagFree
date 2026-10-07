# How it works

```
 your PC                                   your VPS                      game servers
┌──────────────────────────┐   WireGuard  ┌───────────────────────┐
│ ExitLagFree app (no root) │   (UDP)      │ exitlag-node          │      NAT
│   │ local socket / pipe   │ ───────────▶ │  wg-elf interface     │ ───────────▶
│ exitlag-helper (service)  │ only game    │  nftables rules       │
│   TUN + game routes       │ IP ranges    │  HTTPS pairing API    │
└──────────────────────────┘              └───────────────────────┘
```

A relay helps when your ISP's route to the game servers is worse than the route from a well-connected VPS. Your traffic goes to the VPS over WireGuard and leaves from there. Use the app's **Direct vs. via node** check to confirm it helps for your connection.

## The three parts

**Node (`exitlag-node`)** runs on the VPS as a systemd service (or in [Docker](/node/docker)). It manages:
- a WireGuard interface, using the kernel module when available and userspace wireguard-go otherwise;
- NAT and filtering rules (nftables, or iptables as a fallback);
- an HTTPS pairing API with a self-signed certificate that clients pin;
- an authenticated UDP latency probe.

**Helper (`exitlag-helper`)** is a small privileged service on your PC (launchd on macOS, systemd on Linux, a Windows service). It creates the tunnel interface and adds routes only for the selected game ranges. It accepts commands only from the user who installed it.

**App** is the desktop UI. It runs as your normal user and never as administrator. It pairs nodes, measures ping, lets you pick games, connects and disconnects, and compares direct latency with latency through the node.

## Security model: keeping your node yours

- **Invite-only pairing.** `exitlag-node invite` creates a single-use link that expires after 15 minutes by default (7 days at most). The node stores only a SHA-256 hash of the token.
- **Keys never leave the device.** Each device generates its own WireGuard key. The node only learns the public key and gives the device a single `/32` tunnel address.
- **Pinned TLS.** The invite link carries the node's certificate fingerprint, so someone in the middle can't impersonate your node.
- **Quiet to scanners.** WireGuard ignores packets from unknown keys. The latency probe answers only signed, recent packets from paired devices. After 10 failed pairing or authentication attempts, the source IP is locked out for 15 minutes.
- **Revocation and limits.** `exitlag-node devices revoke <id>` takes effect immediately, and `max_devices` caps how many devices can pair.
- **No pivoting.** Paired devices can't reach each other, the VPS itself (apart from ping to the tunnel gateway), or any private, loopback, link-local, CGNAT or multicast range. Setting an [egress allowlist](/node/existing-services#limit-what-devices-can-reach) restricts the node to game ranges only, so it can't be used as a general VPN.

## Limitations

- The tunnel is IPv4-only.
- Split routing is by IP range, not by application. Anything else you run that talks to the same ranges also goes through the node.
- Game IP ranges change over time; profiles include the commands used to refresh them.
