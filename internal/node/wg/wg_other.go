//go:build !linux

package wg

import "errors"

func Setup(Options) (Manager, error) {
	return nil, errors.New("exitlag-node only runs on Linux")
}
