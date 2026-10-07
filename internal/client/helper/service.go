package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kardianos/service"

	"github.com/AltairCA/ExitLagFree/internal/client/ipc"
)

// Config records which local user may control the helper.
type Config struct {
	UID int    `json:"uid,omitempty"` // macOS / Linux
	SID string `json:"sid,omitempty"` // Windows
}

func LoadConfig() (Config, error) {
	var c Config
	b, err := os.ReadFile(ConfigPath)
	if err != nil {
		return c, fmt.Errorf("read %s: %w", ConfigPath, err)
	}
	return c, json.Unmarshal(b, &c)
}

func saveConfig(c Config) error {
	if err := os.MkdirAll(filepath.Dir(ConfigPath), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(ConfigPath, b, 0o644)
}

const systemdUnit = `[Unit]
Description={{Description}}
ConditionFileIsExecutable={{Path | cmdEscape}}
After=network-online.target

[Service]
ExecStart={{Path | cmdEscape}}{{range Arguments}} {{. | cmd}}{{end}}
Restart=always
RestartSec=2
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_CHOWN CAP_DAC_OVERRIDE CAP_FOWNER
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
`

func serviceConfig(executable string) *service.Config {
	return &service.Config{
		Name:        ServiceName,
		DisplayName: "ExitLagFree Helper",
		Description: "Manages the ExitLagFree game tunnel and routes.",
		Executable:  executable,
		Arguments:   []string{"run"},
		Option: service.KeyValue{
			"KeepAlive":     true,
			"RunAtLoad":     true,
			"SystemdScript": systemdUnit,
			"OnFailure":     "restart",
			"StartType":     "automatic",
		},
	}
}

type program struct {
	cancel context.CancelFunc
	done   chan struct{}
	helper *Helper
}

func (p *program) Start(s service.Service) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	l, err := ipc.Listen(accessFor(cfg))
	if err != nil {
		return fmt.Errorf("ipc listen: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done, p.helper = cancel, make(chan struct{}), New()
	go func() {
		defer close(p.done)
		if err := ipc.Serve(ctx, l, p.helper); err != nil {
			log.Printf("helper: ipc: %v", err)
		}
	}()
	log.Printf("helper: listening")
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.helper != nil {
		_ = p.helper.Disconnect()
	}
	if p.cancel != nil {
		p.cancel()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
		}
	}
	return nil
}

// Run runs the helper under the OS service manager (or in the foreground
// when started from a terminal, which is handy for debugging).
func Run() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	s, err := service.New(&program{}, serviceConfig(exe))
	if err != nil {
		return err
	}
	return s.Run()
}

// Install copies the running executable (and any companion files such as
// wintun.dll) into a root-owned directory, records the allowed user and
// registers + starts the service. Running a root service from a
// user-writable path would let any local process escalate, hence the copy.
func Install(cfg Config) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	if src, err = filepath.EvalSymlinks(src); err != nil {
		return err
	}
	_ = Uninstall()

	if err := os.MkdirAll(InstallDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(InstallDir, BinaryName)
	if err := copyFile(src, dst, 0o755); err != nil {
		return fmt.Errorf("copy helper: %w", err)
	}
	for _, f := range extraFiles {
		if err := copyFile(filepath.Join(filepath.Dir(src), f), filepath.Join(InstallDir, f), 0o644); err != nil {
			return fmt.Errorf("copy %s (it must be next to %s): %w", f, filepath.Base(src), err)
		}
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	s, err := service.New(&program{}, serviceConfig(dst))
	if err != nil {
		return err
	}
	if err := s.Install(); err != nil {
		return fmt.Errorf("register service: %w", err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}

// ErrNotInstalled is returned by Upgrade when there is no service to upgrade.
var ErrNotInstalled = errors.New("helper service is not installed")

// Upgrade replaces an installed service with the running executable, keeping
// the recorded allowed user, so installers can update it without knowing who
// that user is.
func Upgrade() error {
	cfg, err := LoadConfig()
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotInstalled
	}
	if err != nil {
		return err
	}
	return Install(cfg)
}

func Uninstall() error {
	s, err := service.New(&program{}, serviceConfig(filepath.Join(InstallDir, BinaryName)))
	if err != nil {
		return err
	}
	_ = s.Stop()
	uninstallErr := s.Uninstall()
	_ = os.RemoveAll(InstallDir)
	_ = os.Remove(ConfigPath)
	return uninstallErr
}

func copyFile(src, dst string, mode os.FileMode) error {
	if same(src, dst) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// On Windows a just-stopped service can hold its executable open briefly.
	for i := 0; ; i++ {
		_ = os.Remove(dst)
		err = os.Rename(tmp, dst)
		if err == nil || i == 40 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

func same(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

var ErrNotElevated = errors.New("this command must be run as root / Administrator")
