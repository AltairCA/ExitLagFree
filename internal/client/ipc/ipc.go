// Package ipc is the local HTTP-over-socket protocol between the desktop app
// (unprivileged) and exitlag-helper (root / LocalSystem). The socket is
// restricted to the user who installed the helper.
package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/AltairCA/ExitLagFree/internal/proto"
)

type Backend interface {
	Status() proto.HelperStatus
	Connect(ctx context.Context, req proto.ConnectRequest) error
	Disconnect() error
	Ping(ctx context.Context, req proto.PingRequest) proto.PingResponse
}

func Handler(b Backend) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, b.Status())
	})
	mux.HandleFunc("POST /v1/connect", func(w http.ResponseWriter, r *http.Request) {
		var req proto.ConnectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, proto.ErrorResponse{Error: "invalid request"})
			return
		}
		if err := b.Connect(r.Context(), req); err != nil {
			writeJSON(w, http.StatusBadRequest, proto.ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, b.Status())
	})
	mux.HandleFunc("POST /v1/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if err := b.Disconnect(); err != nil {
			writeJSON(w, http.StatusInternalServerError, proto.ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, b.Status())
	})
	mux.HandleFunc("POST /v1/ping", func(w http.ResponseWriter, r *http.Request) {
		var req proto.PingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, proto.ErrorResponse{Error: "invalid request"})
			return
		}
		writeJSON(w, http.StatusOK, b.Ping(r.Context(), req))
	})
	return http.MaxBytesHandler(mux, 2<<20)
}

func Serve(ctx context.Context, l net.Listener, b Backend) error {
	srv := &http.Server{Handler: Handler(b), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.Serve(l); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// ErrHelperUnavailable means the helper service isn't installed or running.
var ErrHelperUnavailable = errors.New("helper service is not running")

type Client struct {
	hc *http.Client
}

func NewClient() *Client {
	return &Client{hc: &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return dial(ctx) },
		},
	}}
}

func (c *Client) Status(ctx context.Context) (proto.HelperStatus, error) {
	var out proto.HelperStatus
	return out, c.do(ctx, http.MethodGet, "/v1/status", nil, &out)
}

func (c *Client) Connect(ctx context.Context, req proto.ConnectRequest) (proto.HelperStatus, error) {
	var out proto.HelperStatus
	return out, c.do(ctx, http.MethodPost, "/v1/connect", req, &out)
}

func (c *Client) Disconnect(ctx context.Context) (proto.HelperStatus, error) {
	var out proto.HelperStatus
	return out, c.do(ctx, http.MethodPost, "/v1/disconnect", struct{}{}, &out)
}

func (c *Client) Ping(ctx context.Context, req proto.PingRequest) (proto.PingResponse, error) {
	var out proto.PingResponse
	return out, c.do(ctx, http.MethodPost, "/v1/ping", req, &out)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://helper"+path, body)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		var oe *net.OpError
		if errors.As(err, &oe) && oe.Op == "dial" {
			return fmt.Errorf("%w: %v", ErrHelperUnavailable, err)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e proto.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New(e.Error)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
