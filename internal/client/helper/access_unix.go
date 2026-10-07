//go:build darwin || linux

package helper

import (
	"os"

	"github.com/AltairCA/ExitLagFree/internal/client/ipc"
)

func accessFor(c Config) ipc.Access { return ipc.Access{UID: c.UID} }

func IsElevated() bool { return os.Geteuid() == 0 }
