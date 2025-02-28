// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"net"
	"sync"
)

// sshServerListener is required to prevent a race condition
// that can occur in tests.
//
// The SSH server tracks the listeners in use
// but if the server's close() method executes
// before we reach a safe point in the Serve() method
// then the server's map of listeners will be empty.
// A safe point to indicate the server is ready is
// right before we start accepting connections.
// Accept() will return with error if the underlying
// listener is already closed.
//
// As such, we ensure accept has been called at least once
// before allowing a close to take effect. The corresponding
// piece to this is to receive from the closeAllowed channel
// within your cleanup routine.
type sshServerListener struct {
	net.Listener
	// closeAllowed indicates when the server has reached
	// a safe point that it can be killed.
	closeAllowed chan struct{}
	once         *sync.Once
}

// sshServerListener returns a listener and a closedAllowed channel. You are
// expected to receive from the closeAlloed channel within your Close() function.
// The channel is closed once an accept has occurred at least once.
func newSSHServerListener(l net.Listener) (sshServerListener, chan struct{}) {
	c := make(chan struct{})
	return sshServerListener{
		Listener:     l,
		closeAllowed: c,
		once:         &sync.Once{},
	}, c
}

// Accept runs the listeners accept, but firstly closes the closeAllowed channel,
// signalling that any routines waiting to close the listener may proceed.
func (l sshServerListener) Accept() (net.Conn, error) {
	l.once.Do(func() {
		close(l.closeAllowed)
	})
	return l.Listener.Accept()
}
