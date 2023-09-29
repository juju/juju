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
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
)

type upgradeWatcherSuite struct {
	state  *MockState
	uuidCh chan []string

	w watcher.NotifyWatcher
}

var _ = gc.Suite(&upgradeWatcherSuite{})

func (s *upgradeWatcherSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.uuidCh = make(chan []string)

	uuidWatcher := watchertest.NewMockStringsWatcher(s.uuidCh)
	wf := NewMockWatcherFactory(ctrl)
	wf.EXPECT().NewUUIDsWatcher("upgrade_info", changestream.Create|changestream.Update).Return(uuidWatcher, nil)
	var err error
	s.w, err = NewUpgradeWatcher(context.Background(), s.state, wf, testUUID1)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *upgradeWatcherSuite) TestUpgradeWatcher(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.w.Changes()

	s.state.EXPECT().SelectUpgradeInfo(gomock.Any(), testUUID1).Return(upgrade.Info{
		UUID:            testUUID1,
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		CreatedAt:       time.Now().Add(-10 * time.Minute), // 10 minutes ago
		DBCompletedAt:   time.Now().Add(-5 * time.Minute),  // 5 minutes ago
	}, nil)

	s.uuidCh <- []string{"blah"}
	select {
	case _, ok := <-ch:
		c.Assert(ok, jc.IsTrue)
	case <-time.After(coretesting.ShortWait):
		c.Fatal("Timed out waiting for ready notification")
	}
}
