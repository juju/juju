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

type watcherSuite struct {
	baseServiceSuite

	state *MockState
	nsCh  chan []string

	w watcher.NotifyWatcher
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.nsCh = make(chan []string)

	nsWatcher := watchertest.NewMockStringsWatcher(s.nsCh)
	wf := NewMockWatcherFactory(ctrl)
	wf.EXPECT().NewNamespaceWatcher("upgrade_info_controller_node", changestream.Create|changestream.Update, "").Return(nsWatcher, nil)

	var err error
	s.w, err = NewUpgradeReadyWatcher(context.Background(), s.state, wf, s.upgradeUUID)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *watcherSuite) TestUpgradeReadyWatcherSingleNode(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.w.Changes()

	s.state.EXPECT().AllProvisionedControllersReady(gomock.Any(), s.upgradeUUID).Return(true, nil)

	s.nsCh <- []string{"blah"}
	select {
	case _, ok := <-ch:
		c.Assert(ok, jc.IsTrue)
	case <-time.After(coretesting.ShortWait):
		c.Fatal("Timed out waiting for ready notification")
	}
}

func (s *watcherSuite) TestUpgradeReadyWatcherHA(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.w.Changes()

	s.state.EXPECT().AllProvisionedControllersReady(gomock.Any(), s.upgradeUUID).Return(false, nil).Times(2)
	s.state.EXPECT().AllProvisionedControllersReady(gomock.Any(), s.upgradeUUID).Return(true, nil)

	s.nsCh <- []string{"blah"}
	s.nsCh <- []string{"blah"}
	select {
	case <-ch:
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
