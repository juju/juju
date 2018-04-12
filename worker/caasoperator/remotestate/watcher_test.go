// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator/remotestate"
)

type WatcherSuite struct {
	coretesting.BaseSuite

	watcher     *remotestate.RemoteStateWatcher
	appWatcher  *mockNotifyWatcher
	charmGetter *mockCharmGetter
	clock       *testing.Clock
}

var _ = gc.Suite(&WatcherSuite{})

func (s *WatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.clock = testing.NewClock(time.Now())
	s.appWatcher = newMockNotifyWatcher()
	s.charmGetter = &mockCharmGetter{}
	w, err := remotestate.NewWatcher(remotestate.WatcherConfig{
		Application:        "gitlab",
		ApplicationWatcher: &mockApplicationWatcher{s.appWatcher},
		CharmGetter:        s.charmGetter,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.watcher = w
}

func (s *WatcherSuite) TearDownTest(c *gc.C) {
	if s.watcher != nil {
		s.watcher.Kill()
		err := s.watcher.Wait()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *WatcherSuite) TestInitialSnapshot(c *gc.C) {
	snap := s.watcher.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{})
}

func (s *WatcherSuite) TestInitialSignal(c *gc.C) {
	s.appWatcher.changes <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
}

func (s *WatcherSuite) signalAll() {
	s.appWatcher.changes <- struct{}{}
}

func (s *WatcherSuite) TestRemoteStateChanged(c *gc.C) {
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
