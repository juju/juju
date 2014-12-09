// +build !windows

package sockets

import (
	"net"
	"net/rpc"
	"os"
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
		logger.Errorf("failed to listen on unix:%s: %v", socketPath, err)
		return nil, err
	}
	return listener, err
}
