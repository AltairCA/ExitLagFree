package elevate

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideWindow keeps the console helper from flashing a window when the GUI app runs it.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// runElevated shows the UAC prompt. ShellExecute doesn't report the child's
// exit status, so callers poll the helper afterwards to confirm success.
func runElevated(helper string, args ...string) error {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = syscall.EscapeArg(a)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(helper)
	params, _ := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	if err := windows.ShellExecute(0, verb, file, params, nil, windows.SW_HIDE); err != nil {
		if err == windows.ERROR_CANCELLED {
			return fmt.Errorf("installation cancelled")
		}
		return fmt.Errorf("could not start elevated helper install: %w", err)
	}
	return nil
}

func currentUserSID() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

func InstallHelper(helper string) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	return runElevated(helper, "install", "--sid", sid)
}

func UninstallHelper(helper string) error {
	return runElevated(helper, "uninstall")
}
