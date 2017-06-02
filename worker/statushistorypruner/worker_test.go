// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner_test

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/statushistorypruner"
)

type statusHistoryPrunerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&statusHistoryPrunerSuite{})

func (s *statusHistoryPrunerSuite) setupPruner(c *gc.C) (*fakeFacade, *testing.Clock) {
	facade := newFakeFacade()
	attrs := coretesting.FakeConfig()
	attrs["max-status-history-age"] = "1s"
	attrs["max-status-history-size"] = "3M"
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	facade.modelConfig = cfg

	testClock := testing.NewClock(time.Time{})
	conf := statushistorypruner.Config{
		Facade:        facade,
		PruneInterval: coretesting.ShortWait,
		Clock:         testClock,
	}

	pruner, err := statushistorypruner.New(conf)
	c.Check(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) {
		c.Assert(worker.Stop(pruner), jc.ErrorIsNil)
	})

	facade.changesWatcher.changes <- struct{}{}
	select {
	case <-facade.gotConfig:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for model configr")
	}

	return facade, testClock
}

func (s *statusHistoryPrunerSuite) assertWorkerCallsPrune(c *gc.C, facade *fakeFacade, testClock *testing.Clock, collectionSize int) {
	// NewTimer/Reset will have been called with the PruneInterval.
	testClock.WaitAdvance(coretesting.ShortWait-time.Nanosecond, coretesting.LongWait, 1)
	select {
	case <-facade.pruned:
		c.Fatal("unexpected call to Prune")
	case <-time.After(coretesting.ShortWait):
	}
	testClock.Advance(time.Nanosecond)
	select {
	case args := <-facade.pruned:
		c.Assert(args.maxHistoryMB, gc.Equals, collectionSize)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for call to Prune")
	}
}

func (s *statusHistoryPrunerSuite) TestWorkerCallsPrune(c *gc.C) {
	facade, clock := s.setupPruner(c)
	s.assertWorkerCallsPrune(c, facade, clock, 3)
}

func (s *statusHistoryPrunerSuite) TestWorkerWontCallPruneBeforeFiringTimer(c *gc.C) {
	facade, _ := s.setupPruner(c)

	select {
	case <-facade.pruned:
		c.Fatal("called before firing timer.")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *statusHistoryPrunerSuite) TestModelConfigChange(c *gc.C) {
	facade, clock := s.setupPruner(c)
	s.assertWorkerCallsPrune(c, facade, clock, 3)

	var err error
	facade.modelConfig, err = facade.modelConfig.Apply(map[string]interface{}{"max-status-history-size": "4M"})
	c.Assert(err, jc.ErrorIsNil)
	facade.changesWatcher.changes <- struct{}{}

	s.assertWorkerCallsPrune(c, facade, clock, 4)
}

type mockTimer struct {
	period chan time.Duration
	c      chan time.Time
}

func (t *mockTimer) Reset(d time.Duration) bool {
	select {
	case t.period <- d:
	case <-time.After(coretesting.LongWait):
		panic("timed out waiting for timer to reset")
	}
	return true
}

func (t *mockTimer) CountDown() <-chan time.Time {
	return t.c
}

func (t *mockTimer) fire() error {
	select {
	case t.c <- time.Time{}:
	case <-time.After(coretesting.LongWait):
		return errors.New("timed out waiting for pruner to run")
	}
	return nil
}

func newMockTimer() *mockTimer {
	return &mockTimer{period: make(chan time.Duration, 1),
		c: make(chan time.Time),
	}
}

type fakeFacade struct {
	pruned         chan pruneParams
	changesWatcher *mockNotifyWatcher
	modelConfig    *config.Config
	gotConfig      chan struct{}
}

type pruneParams struct {
	maxAge       time.Duration
	maxHistoryMB int
}

func newFakeFacade() *fakeFacade {
	return &fakeFacade{
		pruned:         make(chan pruneParams, 1),
		gotConfig:      make(chan struct{}, 1),
		changesWatcher: newMockNotifyWatcher(),
	}
}

// Prune implements Facade
func (f *fakeFacade) Prune(maxAge time.Duration, maxHistoryMB int) error {
	select {
	case f.pruned <- pruneParams{maxAge, maxHistoryMB}:
	case <-time.After(coretesting.LongWait):
		return errors.New("timed out waiting for facade call Prune to run")
	}
	return nil
}

// WatchForModelConfigChanges implements Facade
func (f *fakeFacade) WatchForModelConfigChanges() (watcher.NotifyWatcher, error) {
	return f.changesWatcher, nil
}

// ModelConfig implements Facade
func (f *fakeFacade) ModelConfig() (*config.Config, error) {
	f.gotConfig <- struct{}{}
	return f.modelConfig, nil
}

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		stopped: make(chan struct{}),
	}
}

type mockWatcher struct {
	mu      sync.Mutex
	stopped chan struct{}
}

func (w *mockWatcher) Kill() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.Stopped() {
		close(w.stopped)
	}
}

func (w *mockWatcher) Wait() error {
	<-w.stopped
	return nil
}

func (w *mockWatcher) Stopped() bool {
	select {
	case <-w.stopped:
		return true
	default:
		return false
	}
}

func newMockNotifyWatcher() *mockNotifyWatcher {
	return &mockNotifyWatcher{
		mockWatcher: newMockWatcher(),
		changes:     make(chan struct{}, 1),
	}
}

type mockNotifyWatcher struct {
	*mockWatcher
	changes chan struct{}
}

func (w *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}
