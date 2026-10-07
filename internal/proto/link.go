package proto

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const LinkScheme = "elf"

// PairingLink is the invite an operator hands to a device owner:
//
//	elf://<host>:<api-port>?t=<one-time-token>&fp=<sha256-cert-fingerprint>&n=<node-name>
type PairingLink struct {
	Host        string
	Port        int
	Token       string
	Fingerprint string
	NodeName    string
}

func (l PairingLink) String() string {
	q := url.Values{}
	q.Set("t", l.Token)
	q.Set("fp", l.Fingerprint)
	if l.NodeName != "" {
		q.Set("n", l.NodeName)
	}
	u := url.URL{
		Scheme:   LinkScheme,
		Host:     net.JoinHostPort(l.Host, strconv.Itoa(l.Port)),
		RawQuery: q.Encode(),
	}
	return u.String()
}

// APIBase returns the HTTPS base URL of the node control API.
func (l PairingLink) APIBase() string {
	return "https://" + net.JoinHostPort(l.Host, strconv.Itoa(l.Port))
}

func ParsePairingLink(s string) (PairingLink, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return PairingLink{}, fmt.Errorf("invalid link: %w", err)
	}
	if u.Scheme != LinkScheme {
		return PairingLink{}, fmt.Errorf("link must start with %s://", LinkScheme)
	}
	host := u.Hostname()
	if host == "" {
		return PairingLink{}, errors.New("link is missing a host")
	}
	port := DefaultAPIPort
	if p := u.Port(); p != "" {
		port, err = strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return PairingLink{}, errors.New("link has an invalid port")
		}
	}
	q := u.Query()
	l := PairingLink{
		Host:        host,
		Port:        port,
		Token:       q.Get("t"),
		Fingerprint: strings.ToLower(q.Get("fp")),
		NodeName:    q.Get("n"),
	}
	if l.Token == "" {
		return PairingLink{}, errors.New("link is missing the invite token")
	}
	if fp, err := hex.DecodeString(l.Fingerprint); err != nil || len(fp) != 32 {
		return PairingLink{}, errors.New("link has an invalid certificate fingerprint")
	}
	return l, nil
}
