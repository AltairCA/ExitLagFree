// Package api is the node's public HTTPS control API used by clients to
// pair (with a one-time invite) and to query status and latency.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/AltairCA/ExitLagFree/internal/icmpping"
	"github.com/AltairCA/ExitLagFree/internal/netutil"
	"github.com/AltairCA/ExitLagFree/internal/node/store"
	"github.com/AltairCA/ExitLagFree/internal/node/wg"
	"github.com/AltairCA/ExitLagFree/internal/proto"
	"github.com/AltairCA/ExitLagFree/internal/version"
)

type Options struct {
	NodeName        string
	PublicHost      string
	WGPort          int
	ProbePort       int
	MTU             int
	Subnet          netip.Prefix
	Gateway         netip.Addr
	MaxDevices      int
	ServerPublicKey wgtypes.Key
}

type Server struct {
	opts    Options
	store   *store.Store
	stats   func() (map[wgtypes.Key]wg.PeerStats, error)
	changed func()
	ping    func(ctx context.Context, target string) (icmpping.Result, error)

	authFails *limiter
	probeMu   sync.Mutex
	probing   map[string]bool
}

// New builds the API. changed is called after the device set changes so
// the caller can resync WireGuard peers.
func New(opts Options, st *store.Store, stats func() (map[wgtypes.Key]wg.PeerStats, error), changed func()) *Server {
	return &Server{
		opts:      opts,
		store:     st,
		stats:     stats,
		changed:   changed,
		authFails: newLimiter(10, 15*time.Minute, 15*time.Minute),
		probing:   map[string]bool{},
		ping: func(ctx context.Context, target string) (icmpping.Result, error) {
			return icmpping.Ping(ctx, target, 4, 4*time.Second)
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pair", s.handlePair)
	mux.HandleFunc("GET /v1/me", s.auth(s.handleMe))
	mux.HandleFunc("DELETE /v1/me", s.auth(s.handleUnpair))
	mux.HandleFunc("GET /v1/probe", s.auth(s.handleProbe))
	return http.MaxBytesHandler(mux, 8<<10)
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.authFails.Allowed(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}
	var req proto.PairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	dev, token, err := s.store.Redeem(req.Token, req.PublicKey, s.opts.Subnet, s.opts.MaxDevices)
	switch {
	case errors.Is(err, store.ErrInvalidInvite):
		s.authFails.Fail(ip)
		log.Printf("api: rejected pairing attempt from %s", ip)
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	case errors.Is(err, store.ErrBadKey):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, store.ErrDeviceLimit), errors.Is(err, store.ErrKeyInUse), errors.Is(err, store.ErrSubnetFull):
		writeErr(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		log.Printf("api: pair: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	log.Printf("api: paired device %s (%s) from %s as %s", dev.ID, dev.Name, ip, dev.Address)
	s.changed()
	writeJSON(w, http.StatusOK, proto.PairResponse{
		DeviceID:        dev.ID,
		DeviceName:      dev.Name,
		DeviceToken:     token,
		Address:         dev.Address + "/32",
		Gateway:         s.opts.Gateway.String(),
		ServerPublicKey: s.opts.ServerPublicKey.String(),
		Endpoint:        net.JoinHostPort(s.opts.PublicHost, strconv.Itoa(s.opts.WGPort)),
		ProbePort:       s.opts.ProbePort,
		MTU:             s.opts.MTU,
		NodeName:        s.opts.NodeName,
	})
}

func (s *Server) auth(next func(http.ResponseWriter, *http.Request, store.Device)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !s.authFails.Allowed(ip) {
			writeErr(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			s.authFails.Fail(ip)
			writeErr(w, http.StatusUnauthorized, "missing device token")
			return
		}
		dev, ok := s.store.DeviceByToken(token)
		if !ok {
			s.authFails.Fail(ip)
			writeErr(w, http.StatusUnauthorized, "unknown or revoked device")
			return
		}
		next(w, r, dev)
	}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, dev store.Device) {
	resp := proto.MeResponse{
		DeviceID:    dev.ID,
		DeviceName:  dev.Name,
		Address:     dev.Address,
		NodeName:    s.opts.NodeName,
		NodeVersion: version.Version,
	}
	if st, err := s.stats(); err == nil {
		if key, err := wgtypes.ParseKey(dev.PublicKey); err == nil {
			resp.LastHandshake = st[key].LastHandshake
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUnpair(w http.ResponseWriter, r *http.Request, dev store.Device) {
	if err := s.store.Revoke(dev.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	log.Printf("api: device %s (%s) unpaired itself", dev.ID, dev.Name)
	s.changed()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request, dev store.Device) {
	target, err := netip.ParseAddr(r.URL.Query().Get("target"))
	if err != nil || !netutil.IsPublicIPv4(target) {
		writeErr(w, http.StatusBadRequest, "target must be a public IPv4 address")
		return
	}
	s.probeMu.Lock()
	if s.probing[dev.ID] {
		s.probeMu.Unlock()
		writeErr(w, http.StatusTooManyRequests, "a probe is already running for this device")
		return
	}
	s.probing[dev.ID] = true
	s.probeMu.Unlock()
	defer func() {
		s.probeMu.Lock()
		delete(s.probing, dev.ID)
		s.probeMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	res, err := s.ping(ctx, target.String())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "probe failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, proto.ProbeResponse{
		Target:   target.String(),
		RTTMs:    float64(res.RTT.Microseconds()) / 1000,
		LossPct:  res.LossPct,
		Received: res.Received,
		Sent:     res.Sent,
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, proto.ErrorResponse{Error: msg})
}
