// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/caasoperator/remotestate"
)

type WatcherSuite struct {
	coretesting.BaseSuite

	watcher     *remotestate.RemoteStateWatcher
	appWatcher  *mockNotifyWatcher
	charmGetter *mockCharmGetter
	clock       *testclock.Clock
}

var _ = gc.Suite(&WatcherSuite{})

func (s *WatcherSuite) TearDownTest(c *gc.C) {
	if s.watcher != nil {
		s.watcher.Kill()
		err := s.watcher.Wait()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *WatcherSuite) TestInitialSnapshot(c *gc.C) {
	s.setupWatcher(c, nil)
	snap := s.watcher.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{})
}

func (s *WatcherSuite) TestInitialSignal(c *gc.C) {
	s.setupWatcher(c, nil)
	s.appWatcher.changes <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
}

func (s *WatcherSuite) signalAll() {
	s.appWatcher.changes <- struct{}{}
}

func (s *WatcherSuite) TestRemoteStateChanged(c *gc.C) {
	s.setupWatcher(c, nil)

	assertOneChange := func() {
		assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
		assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	}

	curl := charm.MustParseURL("cs:gitlab-4")
	s.charmGetter.curl = curl
	s.charmGetter.version = 666
	s.charmGetter.force = true

	s.signalAll()
	assertOneChange()
	snap := s.watcher.Snapshot()
	c.Assert(snap.ForceCharmUpgrade, jc.IsTrue)
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		CharmModifiedVersion: 666,
		CharmURL:             curl,
		ForceCharmUpgrade:    true,
	})
}

func (s *WatcherSuite) TestApplicationRemovalTerminatesAgent(c *gc.C) {
	s.setupWatcher(c, errors.NotFoundf("app"))
	err := workertest.CheckKilled(c, s.watcher)
	c.Assert(err, gc.Equals, jworker.ErrTerminateAgent)

	// We killed the watcher. Set it to nil
	// so TearDownTest does not reattempt.
	s.watcher = nil
}

func (s *WatcherSuite) setupWatcher(c *gc.C, appWatcherError error) {
	s.clock = testclock.NewClock(time.Now())

	s.appWatcher = newMockNotifyWatcher()
	s.appWatcher.err = appWatcherError
	s.charmGetter = &mockCharmGetter{}

	w, err := remotestate.NewWatcher(remotestate.WatcherConfig{
		Application:        "gitlab",
		ApplicationWatcher: &mockApplicationWatcher{s.appWatcher},
		CharmGetter:        s.charmGetter,
		Logger:             loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.watcher = w
}
