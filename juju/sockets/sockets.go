// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sockets

import (
	"crypto/tls"
	"io"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"

	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.juju.sockets")

// Socket represents the set of parameters to use for socket to dial/listen.
type Socket struct {
	// Network is the socket network.
	Network string

	// Address is the socket address.
	Address string

	// TLSConfig is set when the socket should also establish a TLS connection.
	TLSConfig *tls.Config
}

func Dial(soc Socket) (*rpc.Client, error) {
	var conn io.ReadWriteCloser
	var err error
	if soc.TLSConfig != nil {
		conn, err = tls.Dial(soc.Network, soc.Address, soc.TLSConfig)
	} else {
		conn, err = net.Dial(soc.Network, soc.Address)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return rpc.NewClient(conn), nil
}

func Listen(soc Socket) (net.Listener, error) {
	listener, err := innerListen(soc)
	if err != nil {
		return nil, err
	}
	if soc.TLSConfig != nil {
		return tls.NewListener(listener, soc.TLSConfig), nil
	}
	return listener, nil
}

func innerListen(soc Socket) (listener net.Listener, err error) {
	if soc.Network == "tcp" {
		return net.Listen(soc.Network, soc.Address)
	}
	// In case the unix socket is present, delete it.
	if err := os.Remove(soc.Address); err != nil {
		logger.Tracef("ignoring error on removing %q: %v", soc.Address, err)
	}
	// Listen directly to abstract domain sockets.
	if strings.HasPrefix(soc.Address, "@") {
		listener, err = net.Listen(soc.Network, soc.Address)
		return listener, errors.Trace(err)
	}
	// We first create the socket in a temporary directory as a subdirectory of
	// the target dir so we know we can get the permissions correct and still
	// rename the socket into the correct place.
	// os.MkdirTemp creates the temporary directory as 0700 so it starts with
	// the right perms as well.
	socketDir := filepath.Dir(soc.Address)
	tempdir, err := os.MkdirTemp(socketDir, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer os.RemoveAll(tempdir)
	// Keep the socket path as short as possible so as not to
	// exceed the 108 length limit.
	tempSocketPath := filepath.Join(tempdir, "s")
	listener, err = net.Listen(soc.Network, tempSocketPath)
	if err != nil {
		logger.Errorf("failed to listen on unix:%s: %v", tempSocketPath, err)
		return nil, errors.Trace(err)
	}
	if err := os.Chmod(tempSocketPath, 0700); err != nil {
		listener.Close()
		return nil, errors.Annotatef(err, "could not chmod socket %v", tempSocketPath)
	}
	if err := os.Rename(tempSocketPath, soc.Address); err != nil {
		listener.Close()
		return nil, errors.Annotatef(err, "could not rename socket %v", tempSocketPath)
	}
	return listener, nil
}
