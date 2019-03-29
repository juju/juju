// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/feature"
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

// Current tests for the WatchLXDProfileUpgradeNotifications for machines and
// units.

type instanceCharmProfileWatcherSuite struct {
	watchLXDProfileUpgradeBaseSuite
}

var _ = gc.Suite(&instanceCharmProfileWatcherSuite{})

func (s *instanceCharmProfileWatcherSuite) TestFullWatch(c *gc.C) {
	defer s.setup(c).Finish()

	done := make(chan struct{})
	w := s.workerForScenario(c,
		s.expectInitialCollectionInstanceField(lxdprofile.EmptyStatus),
		s.expectLoopCollectionFilterAndNotify([]watcher.Change{
			{Revno: 0},
			{Revno: 1},
		}, done),
		s.expectLoop,
		s.expectMergeCollectionInstanceField(lxdprofile.NotKnownStatus),
		s.expectMergeCollectionInstanceField(lxdprofile.NotKnownStatus),
		s.expectLoop,
	)

	s.assertChangesSent(c, done, s.close)
	s.assertChanges(c, w, []string{lxdprofile.NotKnownStatus}, noop)
	s.cleanKill(c, w)
	s.assertWatcherChangesClosed(c, w)
}

func (s *instanceCharmProfileWatcherSuite) TestFullWatchWithNoStatusChange(c *gc.C) {
	defer s.setup(c).Finish()

	done := make(chan struct{})
	w := s.workerForScenario(c,
		s.expectInitialCollectionInstanceField(lxdprofile.NotKnownStatus),
		s.expectLoopCollectionFilterAndNotify([]watcher.Change{
			{Revno: 0},
		}, done),
		s.expectLoop,
		s.expectMergeCollectionInstanceField(lxdprofile.NotKnownStatus),
		s.expectLoop,
	)

	s.assertChangesSent(c, done, noop)
	s.assertChanges(c, w, []string{lxdprofile.NotKnownStatus}, noop)
	s.assertNoChanges(c, w)
	s.close()

	s.cleanKill(c, w)
	s.assertWatcherChangesClosed(c, w)
}

func (s *instanceCharmProfileWatcherSuite) workerForScenario(c *gc.C, behaviours ...func()) state.StringsWatcher {
	for _, b := range behaviours {
		b()
	}

	return state.NewInstanceCharmProfileDataWatcher(s.modelBackend, "1")
}

func (s *instanceCharmProfileWatcherSuite) expectInitialCollectionInstanceField(state string) func() {
	return func() {
		s.database.EXPECT().GetCollection("instanceCharmProfileData").Return(s.collection, noop)
		s.collection.EXPECT().Find(bson.D{{"_id", "1"}}).Return(s.query)
		s.query.EXPECT().One(gomock.Any()).SetArg(0, map[string]interface{}{
			"upgradecharmprofilecomplete": state,
		}).Return(nil)
	}
}

func (s *instanceCharmProfileWatcherSuite) expectLoopCollectionFilterAndNotify(changes []watcher.Change, done chan struct{}) func() {
	return func() {
		matcher := channelMatcher{
			changes: changes,
			done:    done,
		}

		s.watcher.EXPECT().WatchCollectionWithFilter("instanceCharmProfileData", matcher, gomock.Any())
		s.watcher.EXPECT().UnwatchCollection("instanceCharmProfileData", gomock.Any())
	}
}

func (s *instanceCharmProfileWatcherSuite) expectMergeCollectionInstanceField(state string) func() {
	return func() {
		s.database.EXPECT().GetCollection("instanceCharmProfileData").Return(s.collection, noop)
		s.collection.EXPECT().Find(bson.D{{"_id", "1"}}).Return(s.query)
		s.query.EXPECT().One(gomock.Any()).SetArg(0, map[string]interface{}{
			"upgradecharmprofilecomplete": state,
		}).Return(nil)
	}
}

func (s *instanceCharmProfileWatcherSuite) assertChangesSent(c *gc.C, done chan struct{}, close func()) {
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
	s.SetInitialFeatureFlags(feature.InstanceMutater)
	s.watchLXDProfileUpgradeBaseSuite.SetUpTest(c)
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) TestFullWatch(c *gc.C) {
	defer s.setup(c).Finish()

	w := s.workerForScenario(c,
		s.expectInitialCollectionInstanceField(charm.MustParseURL("cs:~user/series/name-0")),
		s.expectLoopCollectionFilterAndNotify([]watcher.Change{
			{Revno: 0},
		}),
		s.expectLoop,
		s.expectMergeCollectionInstanceField(charm.MustParseURL("cs:~user/series/name-1")),
		s.expectLoop,
	)

	s.assertChanges(c, w, []string{lxdprofile.NotRequiredStatus}, noop)
	s.assertChanges(c, w, []string{lxdprofile.NotRequiredStatus}, s.close)
	s.cleanKill(c, w)
	s.assertWatcherChangesClosed(c, w)
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) TestFullWatchWithNoStatusChange(c *gc.C) {
	defer s.setup(c).Finish()

	w := s.workerForScenario(c,
		s.expectInitialCollectionInstanceField(charm.MustParseURL("cs:~user/series/name-0")),
		s.expectLoopCollectionFilterAndNotify([]watcher.Change{
			{Revno: 0},
		}),
		s.expectLoop,
		s.expectMergeCollectionInstanceField(charm.MustParseURL("cs:~user/series/name-0")),
		s.expectLoop,
	)

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

func (s *instanceCharmProfileWatcherCompatibilitySuite) expectInitialCollectionInstanceField(url *charm.URL) func() {
	return func() {
		s.database.EXPECT().GetCollection("applications").Return(s.collection, noop)
		s.collection.EXPECT().Find(bson.D{{"_id", "1"}}).Return(s.query)
		s.query.EXPECT().One(gomock.Any()).SetArg(0, map[string]interface{}{
			"charmurl": url,
		}).Return(nil)
	}
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) expectLoopCollectionFilterAndNotify(changes []watcher.Change) func() {
	return func() {
		matcher := channelMatcher{
			changes: changes,
		}

		s.watcher.EXPECT().WatchCollectionWithFilter("applications", matcher, gomock.Any())
		s.watcher.EXPECT().UnwatchCollection("applications", gomock.Any())
	}
}

func (s *instanceCharmProfileWatcherCompatibilitySuite) expectMergeCollectionInstanceField(url *charm.URL) func() {
	return func() {
		s.database.EXPECT().GetCollection("applications").Return(s.collection, noop)
		s.collection.EXPECT().Find(bson.D{{"_id", "1"}}).Return(s.query)
		s.query.EXPECT().One(gomock.Any()).SetArg(0, map[string]interface{}{
			"charmurl": url,
		}).Return(nil)
	}
}

func noop() {
}

// channelMatcher is used here, to not only match on the channel, but also to
// apply values on to the private channel. This isn't pretty, because we don't
// have access to the channel inside the watcher, but serves as an example of
// getting information to private channels.
type channelMatcher struct {
	changes []watcher.Change
	done    chan struct{}
}

func (m channelMatcher) Matches(x interface{}) bool {
	ch, ok := x.(chan<- watcher.Change)
	if ok {
		go func() {
			for _, v := range m.changes {
				ch <- v
			}
			close(m.done)
		}()
		return true
	}
	return false
}

func (channelMatcher) String() string {
	return "is chan watcher.Change"
}
