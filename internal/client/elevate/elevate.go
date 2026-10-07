// Package elevate runs exitlag-helper install/uninstall with a one-time OS
// elevation prompt from the unprivileged desktop app.
package elevate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// HelperVersion asks a helper binary for its version (no elevation needed).
func HelperVersion(helper string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper, "version")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func helperName() string {
	if runtime.GOOS == "windows" {
		return "exitlag-helper.exe"
	}
	return "exitlag-helper"
}

// FindHelper locates the helper binary shipped with the app: next to the
// app executable (also Contents/MacOS inside a macOS bundle), then the
// repo's bin/ directory and PATH for development builds.
func FindHelper() (string, error) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, helperName()),
			filepath.Join(dir, "..", "Resources", helperName()),
			// dev builds: <repo>/app/build/bin[/ExitLagFree.app/Contents/MacOS] -> <repo>/bin
			filepath.Join(dir, "..", "..", "..", "bin", helperName()),
			filepath.Join(dir, "..", "..", "..", "..", "..", "..", "bin", helperName()),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "..", "bin", helperName()))
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return filepath.Clean(c), nil
		}
	}
	if p, err := exec.LookPath(helperName()); err == nil {
		return p, nil
	}
	return "", errors.New("exitlag-helper binary not found next to the app; reinstall ExitLagFree")
}
