//go:build !windows

package elevate

import "os/exec"

func hideWindow(*exec.Cmd) {}
