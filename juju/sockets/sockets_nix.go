// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package sockets

import (
	"net"
	"net/rpc"
	"os"

	"github.com/juju/errors"
)

func Dial(socketPath string) (*rpc.Client, error) {
	return rpc.Dial("unix", socketPath)
}

func Listen(socketPath string) (net.Listener, error) {
	// In case the unix socket is present, delete it.
	if err := os.Remove(socketPath); err != nil {
		logger.Tracef("ignoring error on removing %q: %v", socketPath, err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := os.Chmod(socketPath, 0700); err != nil {
		listener.Close()
		return nil, errors.Trace(err)
	}
	return listener, nil
}
