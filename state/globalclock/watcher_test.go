// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock_test

import (
	// Only used for time types.
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/globalclock"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/workertest"
)

type WatcherSuite struct {
	testing.MgoSuite
	clock  *testing.Clock
	config globalclock.WatcherConfig
}

var _ = gc.Suite(&WatcherSuite{})

const pollInterval = 30 * time.Second

func (s *WatcherSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.Session.DB(database).DropDatabase()
	s.clock = testing.NewClock(time.Time{})
	s.config = globalclock.WatcherConfig{
		Config: globalclock.Config{
			Database:   database,
			Collection: collection,
			Session:    s.Session,
		},
		LocalClock:   s.clock,
		PollInterval: pollInterval,
	}
}

func (s *WatcherSuite) TestNewWatcherValidatesConfigDatabase(c *gc.C) {
	s.config.Database = ""
	_, err := globalclock.NewWatcher(s.config)
	c.Assert(err, gc.ErrorMatches, "missing database")
}

func (s *WatcherSuite) TestNewWatcherValidatesConfigCollection(c *gc.C) {
	s.config.Collection = ""
	_, err := globalclock.NewWatcher(s.config)
	c.Assert(err, gc.ErrorMatches, "missing collection")
}

func (s *WatcherSuite) TestNewWatcherValidatesConfigSession(c *gc.C) {
	s.config.Session = nil
	_, err := globalclock.NewWatcher(s.config)
	c.Assert(err, gc.ErrorMatches, "missing mongo session")
}

func (s *WatcherSuite) TestNewWatcherValidatesConfigLocalClock(c *gc.C) {
	s.config.LocalClock = nil
	_, err := globalclock.NewWatcher(s.config)
	c.Assert(err, gc.ErrorMatches, "missing local clock")
}

func (s *WatcherSuite) TestNewWatcherValidatesConfigPollInterval(c *gc.C) {
	s.config.PollInterval = 0
	_, err := globalclock.NewWatcher(s.config)
	c.Assert(err, gc.ErrorMatches, "missing poll interval")
}

func (s *WatcherSuite) TestNewWatcherInitialValue(c *gc.C) {
	s.writeTime(c, globalEpoch.Add(time.Second))

	w := s.newWatcher(c)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	t := expectChange(c, w)
	c.Assert(t, gc.Equals, globalEpoch.Add(time.Second))
	expectNoChange(c, w)
}

func (s *WatcherSuite) TestNewWatcherInitialValueMissing(c *gc.C) {
	w := s.newWatcher(c)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	t := expectChange(c, w)
	c.Assert(t, gc.Equals, globalEpoch)
	expectNoChange(c, w)
}

func (s *WatcherSuite) TestStopWatcherClosesChannel(c *gc.C) {
	w := s.newWatcher(c)
	c.Assert(w, gc.NotNil)
	workertest.CleanKill(c, w)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsFalse)
	default:
		c.Fatal("expected channel to be closed")
	}
}

func (s *WatcherSuite) TestWatcherPolls(c *gc.C) {
	w := s.newWatcher(c)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	t0 := globalEpoch
	t1 := globalEpoch.Add(time.Second)

	c.Assert(expectChange(c, w), gc.Equals, t0)
	expectNoChange(c, w)

	s.writeTime(c, t1)
	s.clock.WaitAdvance(pollInterval, time.Second, 1)
	c.Assert(expectChange(c, w), gc.Equals, t1)
	expectNoChange(c, w)
}

func (s *WatcherSuite) TestWatcherCoalesces(c *gc.C) {
	w := s.newWatcher(c)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	t0 := globalEpoch
	t1 := globalEpoch.Add(time.Second)
	t2 := globalEpoch.Add(2 * time.Second)

	c.Assert(expectChange(c, w), gc.Equals, t0)
	expectNoChange(c, w)

	s.writeTime(c, t1)
	s.clock.WaitAdvance(pollInterval, time.Second, 1)

	s.writeTime(c, t2)
	s.clock.WaitAdvance(pollInterval, time.Second, 1)

	c.Assert(expectChange(c, w), gc.Equals, t2)
	expectNoChange(c, w)
}

func (s *WatcherSuite) newWatcher(c *gc.C) *globalclock.Watcher {
	w, err := globalclock.NewWatcher(s.config)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *WatcherSuite) writeTime(c *gc.C, t time.Time) {
	coll := s.Session.DB(database).C(collection)
	_, err := coll.UpsertId("g", bson.D{{
		"$set", bson.D{{"time", t.UnixNano()}},
	}})
	c.Assert(err, jc.ErrorIsNil)
}

func expectChange(c *gc.C, w *globalclock.Watcher) time.Time {
	select {
	case t, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		return t
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for change")
	}
	panic("unreachable")
}

func expectNoChange(c *gc.C, w *globalclock.Watcher) {
	select {
	case t := <-w.Changes():
		c.Fatalf("unexpected change: %s", t)
	case <-time.After(coretesting.ShortWait):
	}
}
