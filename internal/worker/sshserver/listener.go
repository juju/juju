// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"net"
	"sync"
)

// syncListener is required to prevent a race condition that can
// occur when closing the SSH server quickly after starting it.
// There is a pending fix upstream https://github.com/gliderlabs/ssh/pull/248
// that will fix this issue, at which point we can delete this file,
// but until then this is a workaround.
//
// The SSH server tracks the listeners in use but if the server's close() method
// executes before we reach a safe point in the Serve() method then the server's
// map of listeners will be empty. A safe point to indicate the server is ready
// is once we've entered the listener's Accept() method. Accept() will return with
// error if the underlying listener is already closed.
type syncListener struct {
	net.Listener
	// closeAllowed indicates when the server has reached
	// a safe point that it can be killed.
	closeAllowed chan struct{}
	once         *sync.Once
}

// newSyncSSHServerListener returns a listener and a read-only
// channel that indicates when the server can be safely closed.
func newSyncSSHServerListener(l net.Listener) (<-chan struct{}, net.Listener) {
	closeAllowed := make(chan struct{})
	return closeAllowed, syncListener{
		Listener:     l,
		closeAllowed: closeAllowed,
		once:         &sync.Once{},
	}
}

// Accept first closes the closeAllowed channel, signalling that
// any routines waiting to close the SSH server may proceed.
// It then runs the listener's Accept() method.
func (l syncListener) Accept() (net.Conn, error) {
	l.once.Do(func() {
		close(l.closeAllowed)
	})
	return l.Listener.Accept()
}

// Close first closes the closeAllowed channel, signalling that
// any routines waiting to close the SSH server may proceed, this is
// done to handle cases where the server was configured incorrectly
// and never reaches a point where it calls Accept().
// It then runs the listener's Close() method.
func (l syncListener) Close() error {
	l.once.Do(func() {
		close(l.closeAllowed)
	})
	return l.Listener.Close()
}
