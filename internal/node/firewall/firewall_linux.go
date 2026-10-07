//go:build linux

package firewall

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Apply enables IPv4 forwarding and installs the ruleset, returning the
// backend used ("nftables" or "iptables").
func Apply(r Rules) (string, error) {
	if err := enableForwarding(); err != nil {
		return "", err
	}
	if hasNFT() {
		cmd := exec.Command("nft", "-f", "-")
		cmd.Stdin = strings.NewReader(r.NFTRuleset())
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("nft: %v: %s", err, bytes.TrimSpace(out))
		}
		applyCompat(r)
		return "nftables", nil
	}
	if _, err := exec.LookPath("iptables"); err != nil {
		return "", fmt.Errorf("neither nft nor iptables is installed")
	}
	for _, args := range r.IPTablesTeardown() {
		_ = exec.Command("iptables", args...).Run()
	}
	for _, args := range r.IPTablesSetup() {
		if out, err := exec.Command("iptables", args...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("iptables %s: %v: %s", strings.Join(args, " "), err, bytes.TrimSpace(out))
		}
	}
	_ = exec.Command("ip6tables", "-I", "FORWARD", "1", "-i", r.Interface, "-j", "DROP").Run()
	return "iptables", nil
}

// Remove tears down whatever Apply installed.
func Remove(r Rules) {
	if hasNFT() {
		_ = exec.Command("nft", "delete", "table", "inet", tableName).Run()
		removeCompat(r)
		return
	}
	for _, args := range r.IPTablesTeardown() {
		_ = exec.Command("iptables", args...).Run()
	}
	_ = exec.Command("ip6tables", "-D", "FORWARD", "-i", r.Interface, "-j", "DROP").Run()
}

const ipForwardPath = "/proc/sys/net/ipv4/ip_forward"

// enableForwarding turns on IPv4 forwarding. /proc/sys is read-only in
// containers, so an already-enabled host setting is accepted as is.
func enableForwarding() error {
	werr := os.WriteFile(ipForwardPath, []byte("1\n"), 0o644)
	if werr == nil {
		return nil
	}
	if b, err := os.ReadFile(ipForwardPath); err == nil && strings.TrimSpace(string(b)) == "1" {
		return nil
	}
	return fmt.Errorf("enable ip_forward: %w (in a container, set net.ipv4.ip_forward=1 on the host)", werr)
}

func hasNFT() bool {
	if _, err := exec.LookPath("nft"); err != nil {
		return false
	}
	return exec.Command("nft", "list", "tables").Run() == nil
}

func applyCompat(r Rules) {
	if _, err := exec.LookPath("iptables"); err != nil {
		return
	}
	for _, rule := range r.CompatAccept() {
		check := append([]string{"-C"}, rule...)
		if exec.Command("iptables", check...).Run() == nil {
			continue
		}
		_ = exec.Command("iptables", append([]string{"-I", rule[0], "1"}, rule[1:]...)...).Run()
	}
}

func removeCompat(r Rules) {
	if _, err := exec.LookPath("iptables"); err != nil {
		return
	}
	for _, rule := range r.CompatAccept() {
		_ = exec.Command("iptables", append([]string{"-D"}, rule...)...).Run()
	}
}
