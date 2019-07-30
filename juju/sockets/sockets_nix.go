// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package sockets

import (
	"io/ioutil"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
)

func Dial(soc Socket) (*rpc.Client, error) {
	return rpc.Dial(soc.Network, soc.Address)
}

func Listen(soc Socket) (listener net.Listener, err error) {
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
	// ioutil.TempDir creates the temporary directory as 0700 so it starts with
	// the right perms as well.
	socketDir := filepath.Dir(soc.Address)
	tempdir, err := ioutil.TempDir(socketDir, "")
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
