// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type listenerSuite struct {
	testhelpers.IsolationSuite

	listener *MockListener
}

var _ = tc.Suite(&listenerSuite{})

func (s *listenerSuite) TestSyncListenerAfterAccept(c *tc.C) {
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

func (s *listenerSuite) TestSyncListenerAfterClose(c *tc.C) {
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

func (s *listenerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.listener = NewMockListener(ctrl)

	return ctrl
}
