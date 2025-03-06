// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
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
//
// Example:
//
//	listener, closeAllowed := newSSHServerListener(s.config.Listener)
//
//	s.tomb.Go(func() error {
//		<-s.tomb.Dying()
//		<-closeAllowed // Not until accept has been called once, will this be allowed to continue.
//		if err := s.Server.Close(); err != nil {
//			...
//		}
//	})
type testingSSHServerListener struct {
	net.Listener
	// closeAllowed indicates when the server has reached
	// a safe point that it can be killed.
	closeAllowed chan struct{}
	once         *sync.Once

	timeout time.Duration
}

// newTestingSSHServerListener returns a listener and a closedAllowed channel.
// You are expected to receive from the closeAllowed channel within your Close()
// function. The channel is closed once an accept has occurred at least once.
func newTestingSSHServerListener(l net.Listener, timeout time.Duration) net.Listener {
	return testingSSHServerListener{
		Listener:     l,
		closeAllowed: make(chan struct{}),
		once:         &sync.Once{},
		timeout:      timeout,
	}
}

// Accept runs the listeners accept, but firstly closes the closeAllowed channel,
// signalling that any routines waiting to close the listener may proceed.
func (l testingSSHServerListener) Accept() (net.Conn, error) {
	l.once.Do(func() {
		close(l.closeAllowed)
	})
	return l.Listener.Accept()
}

func (l testingSSHServerListener) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()

	select {
	case <-l.closeAllowed:
		return l.Listener.Close()
	case <-ctx.Done():
		return ctx.Err()
	}
}

type listenerSuite struct {
	testing.IsolationSuite

	listener *MockListener
}

var _ = gc.Suite(&listenerSuite{})

func (s *listenerSuite) TestAcceptOnceListener(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.listener.EXPECT().Accept().Return(nil, nil)
	s.listener.EXPECT().Close()

	acceptOnceListener := newTestingSSHServerListener(s.listener, time.Second)

	done := make(chan struct{})

	go func() {
		defer close(done)

		// Accept runs and sends down the channel, it is blocked then until
		// Close continues.
		acceptOnceListener.Accept()
	}()

	err := acceptOnceListener.Close()
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fail()
	}
}

func (s *listenerSuite) TestAcceptOnceListenerDoesNotStop(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// No calls to the mock listener should have been made.

	acceptOnceListener := newTestingSSHServerListener(s.listener, time.Millisecond*50)

	err := acceptOnceListener.Close()
	c.Assert(err, jc.ErrorIs, context.DeadlineExceeded)
}

func (s *listenerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.listener = NewMockListener(ctrl)

	return ctrl
}
