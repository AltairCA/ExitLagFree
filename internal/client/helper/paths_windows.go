package helper

import (
	"os"
	"path/filepath"
)

const (
	ServiceName = "ExitLagFreeHelper"
	BinaryName  = "exitlag-helper.exe"
)

var (
	InstallDir = filepath.Join(programFiles(), "ExitLagFree", "Helper")
	ConfigPath = filepath.Join(programData(), "ExitLagFree", "helper.json")

	// wintun.dll must sit next to the helper executable.
	extraFiles = []string{"wintun.dll"}
)

func programFiles() string {
	if p := os.Getenv("ProgramFiles"); p != "" {
		return p
	}
	return `C:\Program Files`
}

func programData() string {
	if p := os.Getenv("ProgramData"); p != "" {
		return p
	}
	return `C:\ProgramData`
}
