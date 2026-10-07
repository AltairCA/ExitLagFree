# Node config file

The node reads `/etc/exitlag-node/config.json` (mode `0600`). `exitlag-node init` creates it; you can also edit it by hand and run `systemctl restart exitlag-node`.

```json
{
  "node_name": "Sydney",
  "public_host": "203.0.113.10",
  "api_port": 8443,
  "wg_port": 51820,
  "probe_port": 51821,
  "interface": "wg-elf",
  "subnet": "10.66.0.0/24",
  "mtu": 1420,
  "max_devices": 10,
  "egress_allowlist": [],
  "data_dir": "/var/lib/exitlag-node",
  "admin_socket": "/run/exitlag-node/admin.sock"
}
```

## Fields

| Field | Default | Rules | Description |
|-------|---------|-------|-------------|
| `node_name` | hostname | | Name shown in the app. |
| `public_host` | (required) | non-empty | Public IP or domain clients connect to. Goes into invite links. |
| `api_port` | `8443` | 1-65535 | TCP port of the HTTPS pairing API. |
| `wg_port` | `51820` | 1-65535, differs from `probe_port` | UDP port for WireGuard. |
| `probe_port` | `51821` | 1-65535 | UDP port for latency probes. |
| `interface` | `wg-elf` | | Name of the WireGuard interface. An existing interface with this name is replaced at startup. |
| `subnet` | `10.66.0.0/24` | IPv4, `/29` or larger | Tunnel addresses. The node uses the first address; devices get the rest. |
| `mtu` | `1420` | 1280-1500 | Tunnel MTU. Lower it (e.g. `1380`) if your VPS network has a smaller MTU, such as some tunnels or PPPoE. |
| `max_devices` | `10` | at least 1 | Maximum paired devices. |
| `egress_allowlist` | empty | IPv4 CIDRs | If set, devices can only reach these ranges. See [Limit what devices can reach](/node/existing-services#limit-what-devices-can-reach). |
| `data_dir` | `/var/lib/exitlag-node` | | Where keys, devices and the TLS certificate are stored. |
| `admin_socket` | `/run/exitlag-node/admin.sock` | | Unix socket used by `invite`, `devices` and `status`. |

## Changing settings after pairing

| Safe to change any time | Requires re-pairing devices |
|-------------------------|-----------------------------|
| `node_name`, `max_devices`, `egress_allowlist` | `public_host`, `api_port`, `wg_port`, `probe_port`, `subnet` |

Devices receive the `mtu` when they pair, so a new MTU only reaches devices paired afterwards.

To re-pair: remove the node in the app, create a new invite and add it again.
