// Package nodeapi is the desktop app's client for a node's control API.
// Every connection is pinned to the certificate fingerprint from the
// pairing link.
package nodeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/AltairCA/ExitLagFree/internal/pin"
	"github.com/AltairCA/ExitLagFree/internal/proto"
)

var ErrRevoked = errors.New("this device was revoked on the node; pair again with a new invite")

type Client struct {
	base  string
	token string
	hc    *http.Client
}

func New(host string, port int, fingerprint, deviceToken string) (*Client, error) {
	hc, err := pin.HTTPClient(fingerprint, 20*time.Second)
	if err != nil {
		return nil, err
	}
	return &Client{
		base:  "https://" + net.JoinHostPort(host, strconv.Itoa(port)),
		token: deviceToken,
		hc:    hc,
	}, nil
}

func Pair(ctx context.Context, link proto.PairingLink, publicKey string) (proto.PairResponse, error) {
	c, err := New(link.Host, link.Port, link.Fingerprint, "")
	if err != nil {
		return proto.PairResponse{}, err
	}
	var out proto.PairResponse
	err = c.do(ctx, http.MethodPost, "/v1/pair", proto.PairRequest{Token: link.Token, PublicKey: publicKey}, &out)
	return out, err
}

func (c *Client) Me(ctx context.Context) (proto.MeResponse, error) {
	var out proto.MeResponse
	return out, c.do(ctx, http.MethodGet, "/v1/me", nil, &out)
}

func (c *Client) Unpair(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/v1/me", nil, nil)
}

func (c *Client) Probe(ctx context.Context, target string) (proto.ProbeResponse, error) {
	var out proto.ProbeResponse
	return out, c.do(ctx, http.MethodGet, "/v1/probe?target="+url.QueryEscape(target), nil, &out)
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
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach node: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e proto.ErrorResponse
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&e)
		if resp.StatusCode == http.StatusUnauthorized && c.token != "" {
			return ErrRevoked
		}
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New(e.Error)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}
