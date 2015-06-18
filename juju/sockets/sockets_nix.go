// +build !windows

package sockets

import (
	"net"
	"os"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

func Dial(socketPath string) (*rpc.Conn, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}
	codec := jsoncodec.NewNet(conn)
	return rpc.NewConn(codec, nil), nil
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
