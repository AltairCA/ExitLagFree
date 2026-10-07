package elevate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func runElevated(helper string, args ...string) error {
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return fmt.Errorf("pkexec not found; run this in a terminal instead:\n  sudo %s %s", helper, joinArgs(args))
	}
	// Root usually can't read a user's AppImage FUSE mount, so stage the
	// helper in a private directory first. `install` copies it to a
	// root-owned location anyway.
	if os.Getenv("APPIMAGE") != "" {
		dir, err := os.MkdirTemp("", "exitlagfree-helper-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		staged := filepath.Join(dir, filepath.Base(helper))
		if err := copyExecutable(helper, staged); err != nil {
			return err
		}
		helper = staged
	}
	out, err := exec.Command(pkexec, append([]string{helper}, args...)...).CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
			return fmt.Errorf("authorization was cancelled or denied")
		}
		return fmt.Errorf("helper install failed: %s", bytes.TrimSpace(out))
	}
	return nil
}

func copyExecutable(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o700)
}

func joinArgs(args []string) string {
	var b bytes.Buffer
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(a)
	}
	return b.String()
}

func InstallHelper(helper string) error {
	return runElevated(helper, "install", "--uid", fmt.Sprint(os.Getuid()))
}

func UninstallHelper(helper string) error {
	return runElevated(helper, "uninstall")
}
