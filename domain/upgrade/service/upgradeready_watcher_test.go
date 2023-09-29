// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
)

type upgradeReadyWatcherSuite struct {
	state *MockState
	nsCh  chan []string

	w watcher.NotifyWatcher
}

var _ = gc.Suite(&upgradeReadyWatcherSuite{})

func (s *upgradeReadyWatcherSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.nsCh = make(chan []string)

	nsWatcher := watchertest.NewMockStringsWatcher(s.nsCh)
	wf := NewMockWatcherFactory(ctrl)
	wf.EXPECT().NewNamespaceWatcher("upgrade_info_controller_node", changestream.Create|changestream.Update, "").Return(nsWatcher, nil)
	var err error
	s.w, err = NewUpgradeReadyWatcher(context.Background(), s.state, wf, testUUID1)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *upgradeReadyWatcherSuite) TestUpgradeReadyWatcherSingleNode(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.w.Changes()

	s.state.EXPECT().AllProvisionedControllersReady(gomock.Any(), testUUID1).Return(true, nil)

	s.nsCh <- []string{"blah"}
	select {
	case _, ok := <-ch:
		c.Assert(ok, jc.IsTrue)
	case <-time.After(coretesting.ShortWait):
		c.Fatal("Timed out waiting for ready notification")
	}
}

func (s *upgradeReadyWatcherSuite) TestUpgradeReadyWatcherHA(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.w.Changes()

	s.state.EXPECT().AllProvisionedControllersReady(gomock.Any(), testUUID1).Return(false, nil).Times(2)
	s.state.EXPECT().AllProvisionedControllersReady(gomock.Any(), testUUID1).Return(true, nil)

	s.nsCh <- []string{"blah"}
	s.nsCh <- []string{"blah"}
	select {
	case _, _ = <-ch:
		c.Fatal("Received unexpected ready notification")
	case <-time.After(coretesting.ShortWait):
	}

	s.nsCh <- []string{"blah"}
	select {
	case _, ok := <-ch:
		c.Assert(ok, jc.IsTrue)
	case <-time.After(coretesting.ShortWait):
		c.Fatal("Timed out waiting for ready notification")
	}
}
