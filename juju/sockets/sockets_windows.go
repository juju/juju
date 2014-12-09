package sockets

import (
	"net"
	"net/rpc"

	"gopkg.in/natefinch/npipe.v2"
)

func Dial(socketPath string) (*rpc.Client, error) {
	conn, err := npipe.Dial(socketPath)
	if err != nil {
		return nil, err
	}
	return rpc.NewClient(conn), nil
}

func Listen(socketPath string) (net.Listener, error) {
	listener, err := npipe.Listen(socketPath)
	if err != nil {
		logger.Errorf("failed to listen on:%s: %v", socketPath, err)
		return nil, err
	}
	return listener, err
}
