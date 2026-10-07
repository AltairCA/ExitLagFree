# Games and routes

Only traffic to the IP ranges of the games you select goes through your node. Everything else uses your normal connection.

Pick games under **Games**, tick any regions, add custom routes if needed, and click **Save selection**. If you're connected, reconnect to apply the change.

## Built-in game profiles

| Game | What gets routed |
|------|------------------|
| Counter-Strike 2 | Valve's game servers and Steam Datagram Relay (AS32590), 14 ranges |
| Valorant | Riot Games' network (Riot Direct, AS6507), 24 ranges |
| League of Legends | Riot Games' network (Riot Direct, AS6507), 24 ranges |
| Fortnite | Amazon EC2 ranges for the matchmaking regions you pick (9 regions) |
| Apex Legends | i3D.net game servers (AS49544, 71 ranges), plus Google Cloud ranges for the "GCE" datacenters you pick (17 regions) |

Profiles are JSON files in the [`profiles/`](https://github.com/AltairCA/ExitLagFree/tree/master/profiles) directory, built into the app.

### Region pickers (Fortnite, Apex Legends)

Some games run on public cloud regions, so the app downloads the provider's published IP list and routes only the regions you tick:
- **Fortnite:** AWS regions, from Amazon's `ip-ranges.json`. You must pick at least one.
- **Apex Legends:** Google Cloud regions, from Google's `cloud.json`. Optional; without any, only the i3D.net ranges are routed.

The lists are cached for 24 hours. Pick only the regions you play in: a cloud region carries lots of unrelated traffic, which would also go through your node.

::: tip Apex Legends datacenters
Apex's in-game datacenter list labels Google Cloud locations with "GCE" (for example "Iowa (GCE)"). Others, including many bare-metal locations, run on i3D.net and are always routed. A few cities use other providers; if a datacenter doesn't improve, look up its server IP and add it as a custom route.
:::

## Custom routes

Add public IPv4 addresses or CIDRs, one per line, for games without a profile or servers a profile misses:

```
203.0.113.50
198.51.100.0/24
```

Private, reserved and overly broad ranges (wider than `/8`) are rejected.

To find a game's server IP while playing:
- **Windows:** Resource Monitor, Network tab, then look at the game's process under "Network Activity" (or `netstat -ano`).
- **macOS / Linux:** `sudo lsof -i -n -P | grep -i <game>`, or `nettop` on macOS.
- Many games show the server IP in a network stats overlay.

## Is the node helping? Direct vs. via node

Enter a game server's IP under **Direct vs. via node** and click **Compare**. The app measures, in parallel:
- **Direct:** ping from your PC to the server over your normal connection;
- **Via node:** ping from your PC to the node, plus ping from the node to the server.

If the via-node total is lower, or more stable, the node helps for that server. Some servers don't answer ping at all; then the comparison can't measure them, but routing still works.

## Choosing where to rent the VPS

Rent the VPS close to the **game servers**, not close to you. Good starting points are the same cities as the datacenters you play on, for example Frankfurt, Virginia, Singapore or Sydney. Compare a couple of providers with the tool above before settling on one.
