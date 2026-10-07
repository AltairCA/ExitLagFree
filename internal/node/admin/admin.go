// Package admin exposes node management (invites, device listing and
// revocation) over a root-only unix socket used by the exitlag-node CLI.
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/AltairCA/ExitLagFree/internal/proto"
)

type Backend interface {
	Invite(name string, ttl time.Duration) (proto.InviteResponse, error)
	Devices() ([]proto.DeviceInfo, error)
	Revoke(id string) error
	Status() proto.NodeStatus
}

func Listen(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

func Handler(b Backend) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /invite", func(w http.ResponseWriter, r *http.Request) {
		var req proto.InviteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		resp, err := b.Invite(req.Name, req.TTL)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, resp)
	})
	mux.HandleFunc("GET /devices", func(w http.ResponseWriter, r *http.Request) {
		ds, err := b.Devices()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, ds)
	})
	mux.HandleFunc("DELETE /devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := b.Revoke(r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, b.Status())
	})
	return mux
}

type Client struct {
	hc *http.Client
}

func NewClient(socket string) *Client {
	return &Client{hc: &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}}
}

func (c *Client) Invite(name string, ttl time.Duration) (proto.InviteResponse, error) {
	var out proto.InviteResponse
	return out, c.do(http.MethodPost, "/invite", proto.InviteRequest{Name: name, TTL: ttl}, &out)
}

func (c *Client) Devices() ([]proto.DeviceInfo, error) {
	var out []proto.DeviceInfo
	return out, c.do(http.MethodGet, "/devices", nil, &out)
}

func (c *Client) Revoke(id string) error {
	return c.do(http.MethodDelete, "/devices/"+id, nil, nil)
}

func (c *Client) Status() (proto.NodeStatus, error) {
	var out proto.NodeStatus
	return out, c.do(http.MethodGet, "/status", nil, &out)
}

func (c *Client) do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, "http://admin"+path, body)
	resp, err := c.hc.Do(req)
	if err != nil {
		var ne *net.OpError
		if errors.As(err, &ne) {
			return fmt.Errorf("cannot reach exitlag-node daemon (is it running, and are you root?): %w", err)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e proto.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return errors.New(e.Error)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(proto.ErrorResponse{Error: err.Error()})
}
