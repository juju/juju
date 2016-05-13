package sockets

import (
	"net"
	"net/rpc"

	"github.com/juju/errors"

	"gopkg.in/natefinch/npipe.v2"
)

func Dial(socketPath string) (*rpc.Client, error) {
	conn, err := npipe.Dial(socketPath)
	return rpc.NewClient(conn), errors.Trace(err)
}

func Listen(socketPath string) (net.Listener, error) {
	listener, err := npipe.Listen(socketPath)
	return listener, errors.Trace(err)
}
