// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows

package sockets

import (
	"context"
	"crypto/tls"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	testing "testing"

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

// Dialer creates a connection based on the provided socket parameters.
func Dialer(soc Socket) (net.Conn, error) {
	var conn net.Conn
	var err error
	if testing.Testing() &&
		soc.Network == "unix" &&
		soc.Address != "" && soc.Address[0] != '@' {
		conn, err = testUnixDial(soc)
	} else if soc.TLSConfig != nil {
		conn, err = tls.Dial(soc.Network, soc.Address, soc.TLSConfig)
	} else {
		conn, err = net.Dial(soc.Network, soc.Address)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conn, nil
}

// Dial creates an RPC client based on the provided socket parameters.
func Dial(soc Socket) (*rpc.Client, error) {
	conn, err := Dialer(soc)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return rpc.NewClient(conn), nil
}

// Listen creates a listener based on the provided socket parameters.
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
		logger.Tracef(context.TODO(), "ignoring error on removing %q: %v", soc.Address, err)
	}
	// Listen directly to abstract domain sockets.
	if strings.HasPrefix(soc.Address, "@") {
		listener, err = net.Listen(soc.Network, soc.Address)
		return listener, errors.Trace(err)
	}

	// If we are testing, listen with long path support.
	if testing.Testing() {
		return testUnixListen(soc)
	}

	// Create the socket in a 0700 temporary directory next to the target,
	// so no other user can connect to it before the Chmod, then rename it
	// into place. This avoids changing the process umask, which is shared
	// by all goroutines.
	tempDir, err := os.MkdirTemp(filepath.Dir(soc.Address), "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	// Keep the socket path as short as possible so as not to
	// exceed the 108 length limit.
	tempSocketPath := filepath.Join(tempDir, "s")
	listener, err = net.Listen(soc.Network, tempSocketPath)
	if err != nil {
		logger.Errorf(context.TODO(), "failed to listen on unix:%s: %v", tempSocketPath, err)
		return nil, errors.Trace(err)
	}
	if err := os.Chmod(tempSocketPath, 0700); err != nil {
		_ = listener.Close()
		return nil, errors.Annotatef(err, "could not chmod socket %v", tempSocketPath)
	}
	if err := os.Rename(tempSocketPath, soc.Address); err != nil {
		_ = listener.Close()
		return nil, errors.Annotatef(err, "could not rename socket %v", tempSocketPath)
	}
	return listener, nil
}

// testUnixListen is used in tests to ensure the socket path is not
// too long for Linux.
func testUnixListen(soc Socket) (listener net.Listener, err error) {
	// Create a directory with a short path to open the socket in.
	// Since wee need to keep the socket path as short as possible
	// so as not to exceed the 108 length limit.
	openSocketDir, err := os.MkdirTemp("", "juju-socket-*")
	if err != nil {
		return nil, errors.Trace(err)
	}
	socketName := filepath.Base(soc.Address)
	if socketName == "." || socketName[0] == filepath.Separator {
		socketName = "juju.socket"
	}
	socketPath := filepath.Join(openSocketDir, socketName)
	listener, err = net.Listen(soc.Network, socketPath)
	if err != nil {
		logger.Errorf(context.TODO(), "failed to listen on unix:%s: %v", socketPath, err)
		_ = os.RemoveAll(openSocketDir)
		return nil, errors.Trace(err)
	}
	if err := os.Chmod(socketPath, 0700); err != nil {
		_ = listener.Close()
		_ = os.RemoveAll(openSocketDir)
		return nil, errors.Annotatef(err, "could not chmod socket %v", socketPath)
	}
	if err := os.Symlink(socketPath, soc.Address); err != nil {
		_ = listener.Close()
		_ = os.RemoveAll(openSocketDir)
		return nil, errors.Annotatef(err, "could not symlink socket %v => %v", socketPath, soc.Address)
	}
	// Wrap the socket listener to ensure cleanup.
	res := &cleanupListener{
		Listener: listener,
		cleanup: func() {
			_ = os.Remove(soc.Address)
			_ = os.RemoveAll(openSocketDir)
		},
	}
	return res, nil
}

// testUnixDial is used during testing to dial a unix socket via
// a symlink indirection. Linux supports dialing a symlink, but
// if the path is too long it will fail with invalid argument.
func testUnixDial(soc Socket) (conn net.Conn, err error) {
	target, err := os.Readlink(soc.Address)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read test unix socket link: %s", soc.Address)
	}
	return net.Dial("unix", target)
}

type cleanupListener struct {
	net.Listener
	cleanup func()
}

func (l *cleanupListener) Close() error {
	err := l.Listener.Close()
	if l.cleanup != nil {
		l.cleanup()
	}
	return err
}
