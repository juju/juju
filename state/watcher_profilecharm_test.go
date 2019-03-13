// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/mocks"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

type watcherCharmProfileSuite struct {
	testing.BaseSuite

	clock      *mocks.MockClock
	database   *mocks.MockDatabase
	collection *mocks.MockCollection
	query      *mocks.MockQuery
	watcher    *mocks.MockBaseWatcher

	modelBackend state.ModelBackendShim
	filter       func(interface{}) bool

	// The done channel is used by tests to indicate that
	// the worker has accomplished the scenario and can be stopped.
	done chan struct{}
	dead chan struct{}
}

var _ = gc.Suite(&watcherCharmProfileSuite{})

func (s *watcherCharmProfileSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.done = make(chan struct{})
	s.dead = make(chan struct{})
}

func (s *watcherCharmProfileSuite) setup(c *gc.C) *gomock.Controller {
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
	s.filter = func(interface{}) bool {
		return true
	}

	return ctrl
}

func (s *watcherCharmProfileSuite) TestFullWatch(c *gc.C) {
	defer s.setup(c).Finish()

	w := s.workerForScenario(c,
		s.expectInitialCollectionInstanceField(state.InstanceCharmProfileData{
			UpgradeCharmProfileComplete: lxdprofile.EmptyStatus,
		}),
		s.expectLoopCollectionFilterAndNotify([]watcher.Change{
			{Revno: 0},
		}),
		s.expectLoop,
		s.expectMergeCollectionInstanceField(state.InstanceCharmProfileData{
			UpgradeCharmProfileComplete: lxdprofile.NotKnownStatus,
		}),
		s.expectLoop,
	)

	for i := 0; i < 2; i++ {
		select {
		case changes, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
			c.Assert(changes, gc.DeepEquals, []string{lxdprofile.NotRequiredStatus})
			if i == 1 {
				close(s.done)
			}
		case <-time.After(testing.LongWait):
			c.Errorf("timed out waiting for watcher changes")
		}
	}

	s.cleanKill(c, w)
	s.assertNoChange(c, w)
}

func (s *watcherCharmProfileSuite) workerForScenario(c *gc.C, behaviours ...func()) state.StringsWatcher {
	for _, b := range behaviours {
		b()
	}

	return state.NewInstanceCharmProfileDataWatcher(s.modelBackend, "1", lxdprofile.NotRequiredStatus, s.filter)
}

// cleanKill waits for notifications to be processed, then waits for the input
// worker to be killed cleanly. If either ops time out, the test fails.
func (s *watcherCharmProfileSuite) cleanKill(c *gc.C, w worker.Worker) {
	select {
	case <-s.done:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
	workertest.CleanKill(c, w)
}

func (s *watcherCharmProfileSuite) assertNoChange(c *gc.C, w state.StringsWatcher) {
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsFalse)
	default:
		c.Fatalf("watcher not closed")
	}
}

func (s *watcherCharmProfileSuite) expectInitialCollectionInstanceField(doc state.InstanceCharmProfileData) func() {
	return func() {
		s.database.EXPECT().GetCollection("instanceCharmProfileData").Return(s.collection, s.noopCloser)
		s.collection.EXPECT().Find(bson.D{{"_id", "1"}}).Return(s.query)
		s.query.EXPECT().One(gomock.Any()).SetArg(0, doc).Return(nil)
	}
}

func (s *watcherCharmProfileSuite) expectLoopCollectionFilterAndNotify(changes []watcher.Change) func() {
	return func() {
		matcher := channelMatcher{
			changes: changes,
		}

		s.watcher.EXPECT().WatchCollectionWithFilter("instanceCharmProfileData", matcher, gomock.Any())
		s.watcher.EXPECT().UnwatchCollection("instanceCharmProfileData", gomock.Any())
	}
}

func (s *watcherCharmProfileSuite) expectMergeCollectionInstanceField(doc state.InstanceCharmProfileData) func() {
	return func() {
		s.database.EXPECT().GetCollection("instanceCharmProfileData").Return(s.collection, s.noopCloser)
		s.collection.EXPECT().Find(bson.D{{"_id", "1"}}).Return(s.query)
		s.query.EXPECT().One(gomock.Any()).SetArg(0, doc).Return(nil)
	}
}

func (s *watcherCharmProfileSuite) expectLoop() {
	s.watcher.EXPECT().Dead().Return(s.dead).AnyTimes()
}

func (s *watcherCharmProfileSuite) noopCloser() {
}

type channelMatcher struct {
	changes []watcher.Change
}

func (m channelMatcher) Matches(x interface{}) bool {
	ch, ok := x.(chan<- watcher.Change)
	if ok {
		go func() {
			for _, v := range m.changes {
				ch <- v
			}
		}()
		return true
	}
	return false
}

func (channelMatcher) String() string {
	return "is chan watcher.Change"
}
