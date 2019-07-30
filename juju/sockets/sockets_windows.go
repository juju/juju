// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sockets

import (
	"net"
	"net/rpc"

	"github.com/juju/errors"
	"gopkg.in/natefinch/npipe.v2"
)

func Dial(soc Socket) (*rpc.Client, error) {
	conn, err := npipe.Dial(soc.Address)
	return rpc.NewClient(conn), errors.Trace(err)
}

func Listen(soc Socket) (net.Listener, error) {
	listener, err := npipe.Listen(soc.Address)
	return listener, errors.Trace(err)
}
