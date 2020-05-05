// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/mocks"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

type watchLXDProfileUpgradeBaseSuite struct {
	testing.BaseSuite

	clock      *mocks.MockClock
	database   *mocks.MockDatabase
	collection *mocks.MockCollection
	query      *mocks.MockQuery
	watcher    *mocks.MockBaseWatcher

	modelBackend state.ModelBackendShim

	// The done channel is used by tests to indicate that
	// the worker has accomplished the scenario and can be stopped.
	done chan struct{}
	dead chan struct{}
}

func (s *watchLXDProfileUpgradeBaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.done = make(chan struct{})
	s.dead = make(chan struct{})
}

func (s *watchLXDProfileUpgradeBaseSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = mocks.NewMockClock(ctrl)
	s.database = mocks.NewMockDatabase(ctrl)
	s.collection = mocks.NewMockCollection(ctrl)
	s.query = mocks.NewMockQuery(ctrl)
	s.watcher = mocks.NewMockBaseWatcher(ctrl)
	s.modelBackend = state.ModelBackendShim{
		Clock:    s.clock,
		Database: s.database,
		Watcher:  s.watcher,
	}

	return ctrl
}

// cleanKill waits for notifications to be processed, then waits for the input
// worker to be killed cleanly. If either ops time out, the test fails.
func (s *watchLXDProfileUpgradeBaseSuite) cleanKill(c *gc.C, w worker.Worker) {
	select {
	case <-s.done:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
	workertest.CleanKill(c, w)
}

func (s *watchLXDProfileUpgradeBaseSuite) close() {
	close(s.done)
}

func (s *watchLXDProfileUpgradeBaseSuite) assertChanges(c *gc.C, w state.StringsWatcher, changes []string, closeFn func()) {
	select {
	case chg, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(chg, gc.DeepEquals, changes)
		closeFn()
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for watcher changes")
	}
}

func (s *watchLXDProfileUpgradeBaseSuite) assertNoChanges(c *gc.C, w state.StringsWatcher) {
	select {
	case <-w.Changes():
		c.Errorf("timed out waiting for watcher changes")
	case <-time.After(testing.ShortWait):
	}
}

func (s *watchLXDProfileUpgradeBaseSuite) assertWatcherChangesClosed(c *gc.C, w state.StringsWatcher) {
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsFalse)
	default:
		c.Fatalf("watcher not closed")
	}
}

func (s *watchLXDProfileUpgradeBaseSuite) expectLoop() {
	s.watcher.EXPECT().Dead().Return(s.dead).AnyTimes()
}

func (s *watchLXDProfileUpgradeBaseSuite) assertChangesSent(c *gc.C, done chan struct{}, close func()) {
	select {
	case <-done:
		close()
	case <-time.After(testing.LongWait):
		c.Errorf("expected watch changes to have been sent")
	}
}

// Compatibility testing the new feature, which essentially watches the
// application for any new changes to the charmURL. If this changes in any way,
// the profile returns lxdprofile.NotRequiredStatus. This is so the uniter lxd
// profile resolver doesn't block waiting for a lxd profile to be installed. The
// function call becomes a no-op.

type instanceCharmProfileWatcherCompatibilitySuite struct {
	watchLXDProfileUpgradeBaseSuite
}

var _ = gc.Suite(&instanceCharmProfileWatcherCompatibilitySuite{})

func (s *instanceCharmProfileWatcherCompatibilitySuite) SetUpTest(c *gc.C) {
	s.watchLXDProfileUpgradeBaseSuite.SetUpTest(c)
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) TestFullWatch(c *gc.C) {
	defer s.setup(c).Finish()

	done := make(chan struct{})
	w := s.workerForScenario(c,
		s.expectInitialCollectionInstanceField("cs:~user/series/name-0"),
		s.expectLoopCollectionFilterAndNotify([]watcher.Change{
			{Revno: -1},
			{Revno: 0},
			{Revno: 1},
		}, done),
		s.expectLoop,
		s.expectMergeCollectionInstanceField("cs:~user/series/name-1"),
		s.expectMergeCollectionInstanceField("cs:~user/series/name-1"),
		s.expectLoop,
	)

	s.assertChangesSent(c, done, s.close)
	s.assertChanges(c, w, []string{lxdprofile.NotRequiredStatus}, noop)
	s.cleanKill(c, w)
	s.assertWatcherChangesClosed(c, w)
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) TestFullWatchWithNoStatusChange(c *gc.C) {
	defer s.setup(c).Finish()

	done := make(chan struct{})
	w := s.workerForScenario(c,
		s.expectInitialCollectionInstanceField("cs:~user/series/name-0"),
		s.expectLoopCollectionFilterAndNotify([]watcher.Change{
			{Revno: 0},
		}, done),
		s.expectLoop,
		s.expectMergeCollectionInstanceField("cs:~user/series/name-0"),
		s.expectLoop,
	)

	s.assertChangesSent(c, done, noop)
	s.assertChanges(c, w, []string{lxdprofile.NotRequiredStatus}, noop)
	s.assertNoChanges(c, w)
	s.close()

	s.cleanKill(c, w)
	s.assertWatcherChangesClosed(c, w)
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) workerForScenario(c *gc.C, behaviours ...func()) state.StringsWatcher {
	for _, b := range behaviours {
		b()
	}

	return state.NewInstanceCharmProfileDataCompatibilityWatcher(s.modelBackend, "1")
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) expectInitialCollectionInstanceField(url string) func() {
	return func() {
		s.database.EXPECT().GetCollection("applications").Return(s.collection, noop)
		s.collection.EXPECT().Find(bson.D{{"_id", "1"}}).Return(s.query)
		s.query.EXPECT().One(gomock.Any()).SetArg(0, state.ApplicationDoc{
			CharmURL: charm.MustParseURL(url),
		}).Return(nil)
	}
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) expectLoopCollectionFilterAndNotify(changes []watcher.Change, done chan struct{}) func() {
	return func() {
		do := func(collection string, ch chan<- watcher.Change, filter func(interface{}) bool) {
			go func() {
				for _, change := range changes {
					ch <- change
				}
				done <- struct{}{}
			}()
		}
		s.watcher.EXPECT().WatchCollectionWithFilter("applications", gomock.Any(), gomock.Any()).Do(do)
		s.watcher.EXPECT().UnwatchCollection("applications", gomock.Any())
	}
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) expectMergeCollectionInstanceField(url string) func() {
	return func() {
		s.database.EXPECT().GetCollection("applications").Return(s.collection, noop)
		s.collection.EXPECT().Find(bson.D{{"_id", "1"}}).Return(s.query)
		s.query.EXPECT().One(gomock.Any()).SetArg(0, state.ApplicationDoc{
			CharmURL: charm.MustParseURL(url),
		}).Return(nil)
	}
}

func noop() {
}
