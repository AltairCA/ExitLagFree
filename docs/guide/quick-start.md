# Quick start

This gets you from nothing to a working relay in about five minutes.

## What you need

- A Linux VPS (Ubuntu or Debian recommended, amd64 or arm64) in the same city or region as the game servers you play on. The smallest plan is plenty.
- Root (sudo) access to it.
- The desktop app for macOS, Windows or Linux.

::: tip Already running other things on that VPS?
The installer opens ports in ufw or firewalld and adds its own firewall table, but it doesn't touch your existing rules. If you'd rather control every step, or the default ports are taken, follow the [manual install](/node/manual-install) or use [Docker](/node/docker) instead. [Servers with existing services](/node/existing-services) explains how the node coexists with other software.
:::

## 1. Set up the node

SSH into the VPS and run:

```bash
curl -fsSL https://github.com/AltairCA/ExitLagFree/releases/latest/download/install.sh | sudo bash
```

When it finishes, it prints a one-time pairing link that starts with `elf://` (and a QR code). Keep that terminal open.

If your provider has a **cloud firewall or security group**, allow these inbound ports there too:

| Port      | Purpose       |
|-----------|---------------|
| TCP 8443  | Pairing API   |
| UDP 51820 | WireGuard     |
| UDP 51821 | Latency probe |

See [One-command install](/node/install-script) for the installer's options and exactly what it changes.

## 2. Install the desktop app

Download it from the [releases page](https://github.com/AltairCA/ExitLagFree/releases) and follow [Install the app](/app/install). On first launch click **Install helper** and approve the password or UAC prompt. You only do this once.

## 3. Pair and connect

1. Click **+ Add node** and paste the `elf://` link.
2. Pick your games under **Games** and click **Save selection**.
3. Click **Connect**.

Use **Direct vs. via node** with a game server IP to check that the node actually lowers your ping before you play.

## Next steps

- Invite another PC: `sudo exitlag-node invite --name "laptop"`. See [Managing your node](/node/managing).
- Learn what's routed for each game in [Games and routes](/app/games).
