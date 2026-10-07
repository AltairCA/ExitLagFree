package elevate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func runElevated(helper string, args ...string) error {
	cmd := "quoted form of " + appleScriptString(helper)
	for _, a := range args {
		cmd += ` & " " & quoted form of ` + appleScriptString(a)
	}
	script := fmt.Sprintf(`do shell script (%s) with prompt "ExitLagFree needs to install its network helper." with administrator privileges`, cmd)
	out, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		msg := string(bytes.TrimSpace(out))
		if strings.Contains(msg, "-128") {
			return fmt.Errorf("installation cancelled")
		}
		return fmt.Errorf("helper install failed: %s", msg)
	}
	return nil
}

func InstallHelper(helper string) error {
	return runElevated(helper, "install", "--uid", fmt.Sprint(os.Getuid()))
}

func UninstallHelper(helper string) error {
	return runElevated(helper, "uninstall")
}
