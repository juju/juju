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

func Dial(socketPath string) (*rpc.Client, error) {
	return rpc.Dial("unix", socketPath)
}

func Listen(socketPath string) (net.Listener, error) {
	// In case the unix socket is present, delete it.
	if err := os.Remove(socketPath); err != nil {
		logger.Tracef("ignoring error on removing %q: %v", socketPath, err)
	}
	if strings.HasPrefix(socketPath, "@") {
		listener, err := net.Listen("unix", socketPath)
		return listener, errors.Trace(err)
	}
	// Listen directly to abstract domain sockets.
	// We first create the socket in a temporary directory as a subdirectory of
	// the target dir so we know we can get the permissions correct and still
	// rename the socket into the correct place.
	// ioutil.TempDir creates the temporary directory as 0700 so it starts with
	// the right perms as well.
	socketDir := filepath.Dir(socketPath)
	tempdir, err := ioutil.TempDir(socketDir, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer os.RemoveAll(tempdir)
	// Keep the socket path as short as possible so as not to
	// exceed the 108 length limit.
	tempSocketPath := filepath.Join(tempdir, "s")
	listener, err := net.Listen("unix", tempSocketPath)
	if err != nil {
		logger.Errorf("failed to listen on unix:%s: %v", tempSocketPath, err)
		return nil, errors.Trace(err)
	}
	if err := os.Chmod(tempSocketPath, 0700); err != nil {
		listener.Close()
		return nil, errors.Annotatef(err, "could not chmod socket %v", tempSocketPath)
	}
	if err := os.Rename(tempSocketPath, socketPath); err != nil {
		listener.Close()
		return nil, errors.Annotatef(err, "could not rename socket %v", tempSocketPath)
	}
	return listener, nil
}
