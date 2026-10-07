//go:build !linux

package firewall

import "errors"

func Apply(Rules) (string, error) { return "", errors.New("firewall management requires Linux") }

func Remove(Rules) {}
