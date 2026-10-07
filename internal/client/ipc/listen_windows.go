package ipc

import (
	"context"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const PipeName = `\\.\pipe\exitlag-helper`

// Access describes who may talk to the helper.
type Access struct {
	SID string
}

// Listen creates a named pipe whose DACL grants access only to SYSTEM,
// Administrators and the user who installed the helper.
func Listen(a Access) (net.Listener, error) {
	if _, err := windows.StringToSid(a.SID); err != nil {
		return nil, fmt.Errorf("invalid allowed SID %q: %w", a.SID, err)
	}
	sddl := fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;%s)", a.SID)
	return winio.ListenPipe(PipeName, &winio.PipeConfig{
		SecurityDescriptor: sddl,
		InputBufferSize:    64 << 10,
		OutputBufferSize:   64 << 10,
	})
}

func dial(ctx context.Context) (net.Conn, error) {
	c, err := winio.DialPipeContext(ctx, PipeName)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: "pipe", Err: err}
	}
	return c, nil
}
