package helper

import (
	"golang.org/x/sys/windows"

	"github.com/AltairCA/ExitLagFree/internal/client/ipc"
)

func accessFor(c Config) ipc.Access { return ipc.Access{SID: c.SID} }

func IsElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }
