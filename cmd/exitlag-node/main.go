// Command exitlag-node turns a Linux VPS into a private game-traffic relay.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/mdp/qrterminal/v3"

	"github.com/AltairCA/ExitLagFree/internal/node"
	"github.com/AltairCA/ExitLagFree/internal/node/admin"
	"github.com/AltairCA/ExitLagFree/internal/version"
)

const usage = `exitlag-node - self-hosted game traffic relay

Usage:
  exitlag-node init --public-host <ip-or-domain> [options]   write config
  exitlag-node serve                                          run the daemon
  exitlag-node invite [--name NAME] [--ttl 15m] [--qr]        create a one-time pairing link
  exitlag-node devices [list]                                 list paired devices
  exitlag-node devices revoke <id>                            remove a device
  exitlag-node status                                         show node status
  exitlag-node version

All commands accept --config (default /etc/exitlag-node/config.json).
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "init":
		err = cmdInit(args)
	case "serve":
		err = cmdServe(args)
	case "invite":
		err = cmdInvite(args)
	case "devices":
		err = cmdDevices(args)
	case "status":
		err = cmdStatus(args)
	case "version", "--version":
		fmt.Println(version.Version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	cfgPath := fs.String("config", node.DefaultConfigPath, "config file path")
	def := node.DefaultConfig()
	host := fs.String("public-host", "", "public IP or domain clients use to reach this VPS (required)")
	name := fs.String("name", def.NodeName, "display name for this node")
	apiPort := fs.Int("api-port", def.APIPort, "TCP port of the HTTPS control API")
	wgPort := fs.Int("wg-port", def.WGPort, "UDP port for WireGuard")
	probePort := fs.Int("probe-port", def.ProbePort, "UDP port for latency probes")
	subnet := fs.String("subnet", def.Subnet, "tunnel subnet")
	maxDevices := fs.Int("max-devices", def.MaxDevices, "maximum paired devices")
	allow := fs.String("egress-allowlist", "", "comma-separated CIDRs peers may reach (empty = any public address)")
	force := fs.Bool("force", false, "overwrite an existing config")
	fs.Parse(args)

	if _, err := os.Stat(*cfgPath); err == nil && !*force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", *cfgPath)
	}
	cfg := def
	cfg.PublicHost = strings.TrimSpace(*host)
	cfg.NodeName = *name
	cfg.APIPort, cfg.WGPort, cfg.ProbePort = *apiPort, *wgPort, *probePort
	cfg.Subnet = *subnet
	cfg.MaxDevices = *maxDevices
	if *allow != "" {
		for _, c := range strings.Split(*allow, ",") {
			if c = strings.TrimSpace(c); c != "" {
				cfg.EgressAllowlist = append(cfg.EgressAllowlist, c)
			}
		}
	}
	if err := node.SaveConfig(*cfgPath, cfg); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", *cfgPath)
	return nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", node.DefaultConfigPath, "config file path")
	fs.Parse(args)
	if os.Geteuid() != 0 {
		return errors.New("serve must run as root (it manages network interfaces and firewall rules)")
	}
	cfg, err := node.LoadConfig(*cfgPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return node.Run(ctx, cfg)
}

func adminClient(fs *flag.FlagSet, args []string) (*admin.Client, error) {
	cfgPath := fs.String("config", node.DefaultConfigPath, "config file path")
	fs.Parse(args)
	socket := node.DefaultAdminSocket
	if cfg, err := node.LoadConfig(*cfgPath); err == nil && cfg.AdminSocket != "" {
		socket = cfg.AdminSocket
	}
	return admin.NewClient(socket), nil
}

func cmdInvite(args []string) error {
	fs := flag.NewFlagSet("invite", flag.ExitOnError)
	name := fs.String("name", "device", "name for the device that will use this invite")
	ttl := fs.Duration("ttl", node.DefaultInviteTTL, "how long the invite stays valid")
	qr := fs.Bool("qr", false, "also print the link as a QR code")
	c, err := adminClient(fs, args)
	if err != nil {
		return err
	}
	inv, err := c.Invite(*name, *ttl)
	if err != nil {
		return err
	}
	fmt.Printf("One-time pairing link for %q (expires %s):\n\n  %s\n\n", *name, inv.ExpiresAt.Local().Format(time.RFC1123), inv.Link)
	if *qr {
		qrterminal.GenerateHalfBlock(inv.Link, qrterminal.L, os.Stdout)
		fmt.Println()
	}
	fmt.Println("Paste it into ExitLagFree > Add node. It works once; share it privately.")
	return nil
}

func cmdDevices(args []string) error {
	sub := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "list", "ls":
		c, err := adminClient(flag.NewFlagSet("devices list", flag.ExitOnError), args)
		if err != nil {
			return err
		}
		ds, err := c.Devices()
		if err != nil {
			return err
		}
		if len(ds) == 0 {
			fmt.Println("no paired devices; create one with `exitlag-node invite --name <device>`")
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tNAME\tADDRESS\tLAST HANDSHAKE\tRX\tTX\tENDPOINT")
		for _, d := range ds {
			hs := "never"
			if !d.LastHandshake.IsZero() {
				hs = time.Since(d.LastHandshake).Round(time.Second).String() + " ago"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", d.ID, d.Name, d.Address, hs, bytes(d.RxBytes), bytes(d.TxBytes), d.Endpoint)
		}
		return tw.Flush()
	case "revoke", "rm":
		fs := flag.NewFlagSet("devices revoke", flag.ExitOnError)
		var id string
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			id, args = args[0], args[1:]
		}
		c, err := adminClient(fs, args)
		if err != nil {
			return err
		}
		if id == "" {
			id = fs.Arg(0)
		}
		if id == "" {
			return errors.New("usage: exitlag-node devices revoke <id>")
		}
		if err := c.Revoke(id); err != nil {
			return err
		}
		fmt.Printf("revoked %s; it can no longer connect\n", id)
		return nil
	default:
		return fmt.Errorf("unknown devices subcommand %q", sub)
	}
}

func cmdStatus(args []string) error {
	c, err := adminClient(flag.NewFlagSet("status", flag.ExitOnError), args)
	if err != nil {
		return err
	}
	s, err := c.Status()
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "Node:\t%s (%s)\n", s.NodeName, s.Version)
	fmt.Fprintf(tw, "Public host:\t%s\n", s.PublicHost)
	fmt.Fprintf(tw, "WireGuard:\t%s (%s)\n", s.Interface, s.WGBackend)
	fmt.Fprintf(tw, "Firewall:\t%s\n", s.Firewall)
	fmt.Fprintf(tw, "Devices:\t%d / %d\n", s.Devices, s.MaxDevices)
	fmt.Fprintf(tw, "Pending invites:\t%d\n", s.PendingInvites)
	fmt.Fprintf(tw, "Cert fingerprint:\t%s\n", s.Fingerprint)
	return tw.Flush()
}

func bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
