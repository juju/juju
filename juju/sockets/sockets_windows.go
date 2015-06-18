package sockets

import (
	"net"

	"gopkg.in/natefinch/npipe.v2"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

func Dial(socketPath string) (*rpc.Conn, error) {
	conn, err := npipe.Dial(socketPath)
	if err != nil {
		return nil, err
	}
	codec := jsoncodec.NewNet(conn)
	return rpc.NewConn(codec, nil), nil
}

func Listen(socketPath string) (net.Listener, error) {
	listener, err := npipe.Listen(socketPath)
	if err != nil {
		logger.Errorf("failed to listen on:%s: %v", socketPath, err)
		return nil, err
	}
	return listener, err
}
