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
	return listener, errors.Trace(err)
}
