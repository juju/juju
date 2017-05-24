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
	// We first create the socket in a temporary directory as a subdirectory of
	// the target dir so we know we can get the permissions correct and still
	// rename the socket into the correct place.
	// ioutil.TempDir creates the temporary directory as 0700 so it starts with
	// the right perms as well.
	socketDir, socketName := filepath.Split(socketPath)
	// socketName here is just the prefix for the temporary dir name,
	// so it won't collide
	tempdir, err := ioutil.TempDir(socketDir, socketName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer os.RemoveAll(tempdir)
	tempSocketPath := filepath.Join(tempdir, socketName)
	listener, err := net.Listen("unix", tempSocketPath)
	if err != nil {
		logger.Errorf("failed to listen on unix:%s: %v", socketPath, err)
		return nil, errors.Trace(err)
	}
	if err := os.Chmod(tempSocketPath, 0700); err != nil {
		listener.Close()
		return nil, errors.Trace(err)
	}
	if err := os.Rename(tempSocketPath, socketPath); err != nil {
		listener.Close()
		return nil, errors.Trace(err)
	}
	return listener, nil
}
