---
layout: home

hero:
  name: ExitLagFree
  text: Your own game accelerator
  tagline: Rent a cheap Linux VPS near the game servers, turn it into a private relay, and send only your game traffic through it over WireGuard.
  image:
    src: /screenshot.png
    alt: ExitLagFree desktop app
  actions:
    - theme: brand
      text: Quick start
      link: /guide/quick-start
    - theme: alt
      text: Server already in use? Manual install
      link: /node/manual-install
    - theme: alt
      text: Download
      link: https://github.com/AltairCA/ExitLagFree/releases

features:
  - title: Only game traffic
    details: Split routing sends just the selected games' IP ranges through your node. Browsing, downloads and voice chat keep using your normal connection.
  - title: Nobody else can use your node
    details: One-time invite links, a pinned TLS certificate, per-device keys, instant revocation and firewall rules that stop paired devices from reaching anything private.
  - title: Fits on servers you already run
    details: Use the one-command installer, set it up by hand next to existing services, or run it in Docker.
---
