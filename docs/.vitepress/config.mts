import { defineConfig } from "vitepress";

const repo = "https://github.com/AltairCA/ExitLagFree";

export default defineConfig({
  title: "ExitLagFree",
  description: "Self-hosted game traffic relay: route only your game traffic through your own VPS over WireGuard.",
  base: "/ExitLagFree/",
  cleanUrls: true,
  lastUpdated: true,
  head: [["meta", { name: "theme-color", content: "#3b82f6" }]],

  themeConfig: {
    nav: [
      { text: "Guide", link: "/guide/quick-start" },
      { text: "Node (VPS)", link: "/node/install-script" },
      { text: "Desktop app", link: "/app/install" },
      { text: "Reference", link: "/reference/node-cli" },
      { text: "Downloads", link: `${repo}/releases` },
    ],

    sidebar: [
      {
        text: "Guide",
        items: [
          { text: "Quick start", link: "/guide/quick-start" },
          { text: "How it works", link: "/guide/how-it-works" },
        ],
      },
      {
        text: "Node (VPS)",
        items: [
          { text: "One-command install", link: "/node/install-script" },
          { text: "Manual install", link: "/node/manual-install" },
          { text: "Servers with existing services", link: "/node/existing-services" },
          { text: "Docker", link: "/node/docker" },
          { text: "Managing your node", link: "/node/managing" },
        ],
      },
      {
        text: "Desktop app",
        items: [
          { text: "Install the app", link: "/app/install" },
          { text: "Games and routes", link: "/app/games" },
        ],
      },
      {
        text: "Reference",
        items: [
          { text: "exitlag-node CLI", link: "/reference/node-cli" },
          { text: "Node config file", link: "/reference/node-config" },
        ],
      },
      { text: "Troubleshooting", link: "/troubleshooting" },
    ],

    search: { provider: "local" },
    editLink: {
      pattern: `${repo}/edit/master/docs/:path`,
      text: "Edit this page on GitHub",
    },
    socialLinks: [{ icon: "github", link: repo }],
    footer: {
      message: "Open source. Your node, your traffic.",
    },
  },
});
