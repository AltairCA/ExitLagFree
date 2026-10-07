//go:build !windows

package icmpping

import "os"

func isRoot() bool { return os.Geteuid() == 0 }
