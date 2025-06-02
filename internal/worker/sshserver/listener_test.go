// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"time"

	"github.com/juju/testing"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type listenerSuite struct {
	testing.IsolationSuite

	listener *MockListener
}

var _ = gc.Suite(&listenerSuite{})

func (s *listenerSuite) TestSyncListenerAfterAccept(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.listener.EXPECT().Accept().Return(nil, nil)

	closeAllowed, syncListener := newSyncSSHServerListener(s.listener)

	select {
	case <-closeAllowed:
		c.Error("closeAllowed channel should not be closed yet")
	case <-time.After(testing.ShortWait):
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		// Accept runs and signals the server can now be closed.
		_, _ = syncListener.Accept()
	}()

	<-done
	select {
	case <-closeAllowed:
	case <-time.After(testing.ShortWait):
		c.Fail()
	}
}

func (s *listenerSuite) TestSyncListenerAfterClose(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.listener.EXPECT().Close().Return(nil)

	closeAllowed, syncListener := newSyncSSHServerListener(s.listener)

	select {
	case <-closeAllowed:
		c.Error("closeAllowed channel should not be closed yet")
	case <-time.After(testing.ShortWait):
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		// Close runs and signals the server can now be closed.
		_ = syncListener.Close()
	}()

	<-done
	select {
	case <-closeAllowed:
	case <-time.After(testing.ShortWait):
		c.Fail()
	}
}

func (s *listenerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.listener = NewMockListener(ctrl)

	return ctrl
}
