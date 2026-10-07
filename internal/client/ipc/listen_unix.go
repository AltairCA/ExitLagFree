//go:build darwin || linux

package ipc

import (
	"context"
	"log"
	"net"
	"os"
)

const SocketPath = "/var/run/exitlag-helper.sock"

// Access describes who may talk to the helper.
type Access struct {
	UID int
}

// Listen creates the helper socket owned by the allowed user with mode
// 0600, and additionally verifies each peer's UID via kernel credentials.
func Listen(a Access) (net.Listener, error) {
	_ = os.Remove(SocketPath)
	l, err := net.Listen("unix", SocketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chown(SocketPath, a.UID, -1); err != nil {
		l.Close()
		return nil, err
	}
	if err := os.Chmod(SocketPath, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return &credListener{Listener: l, uid: a.UID}, nil
}

type credListener struct {
	net.Listener
	uid int
}

func (l *credListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		uid, err := peerUID(c.(*net.UnixConn))
		if err == nil && (uid == l.uid || uid == 0) {
			return c, nil
		}
		log.Printf("ipc: rejected connection from uid %d (err=%v)", uid, err)
		c.Close()
	}
}

func dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", SocketPath)
}
