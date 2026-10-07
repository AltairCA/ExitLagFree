// Command exitlag-helper is the privileged companion of the ExitLagFree
// desktop app. It owns the tunnel interface and routes so the app itself
// never runs as root / Administrator.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/AltairCA/ExitLagFree/internal/client/helper"
	"github.com/AltairCA/ExitLagFree/internal/client/ipc"
	"github.com/AltairCA/ExitLagFree/internal/version"
)

const usage = `exitlag-helper - privileged tunnel service for ExitLagFree

Usage:
  exitlag-helper install --uid <uid>     (macOS/Linux, as root)
  exitlag-helper install --sid <sid>     (Windows, as Administrator)
  exitlag-helper upgrade                 replace an installed service with this binary, keeping its settings
  exitlag-helper uninstall
  exitlag-helper run                     run the service (used by the service manager)
  exitlag-helper status                  query the running service
  exitlag-helper version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "install":
		err = cmdInstall(os.Args[2:])
	case "upgrade":
		if !helper.IsElevated() {
			err = helper.ErrNotElevated
			break
		}
		err = helper.Upgrade()
		if errors.Is(err, helper.ErrNotInstalled) {
			fmt.Println("helper service is not installed; nothing to upgrade")
			err = nil
		} else if err == nil {
			fmt.Println("helper upgraded to", version.Version)
		}
	case "uninstall":
		if !helper.IsElevated() {
			err = helper.ErrNotElevated
			break
		}
		err = helper.Uninstall()
		if err == nil {
			fmt.Println("helper uninstalled")
		}
	case "run":
		err = helper.Run()
	case "status":
		err = cmdStatus()
	case "version", "--version":
		fmt.Println(version.Version)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	uid := fs.Int("uid", -1, "UID of the user allowed to control the helper")
	sid := fs.String("sid", "", "SID of the user allowed to control the helper (Windows)")
	fs.Parse(args)

	if !helper.IsElevated() {
		return helper.ErrNotElevated
	}
	cfg := helper.Config{UID: *uid, SID: *sid}
	if runtime.GOOS == "windows" {
		if cfg.SID == "" {
			return fmt.Errorf("--sid is required on Windows")
		}
	} else {
		if cfg.UID < 0 {
			if s := os.Getenv("SUDO_UID"); s != "" {
				fmt.Sscanf(s, "%d", &cfg.UID)
			}
		}
		if cfg.UID < 0 {
			return fmt.Errorf("--uid is required")
		}
	}
	if err := helper.Install(cfg); err != nil {
		return err
	}
	fmt.Println("helper installed and started")
	return nil
}

func cmdStatus() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := ipc.NewClient().Status(ctx)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(st)
}
