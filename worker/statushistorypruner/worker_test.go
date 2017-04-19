// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner_test

import (
	"sync"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/statushistorypruner"
)

type statusHistoryPrunerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&statusHistoryPrunerSuite{})

func (s *statusHistoryPrunerSuite) setupPruner(c *gc.C) (*fakeFacade, *mockTimer) {
	fakeTimer := newMockTimer()

	fakeTimerFunc := func(d time.Duration) jworker.PeriodicTimer {
		// construction of timer should be with 0 because we intend it to
		// run once before waiting.
		c.Assert(d, gc.Equals, 0*time.Nanosecond)
		return fakeTimer
	}
	facade := newFakeFacade()
	attrs := coretesting.FakeConfig()
	attrs["max-status-history-age"] = "1s"
	attrs["max-status-history-size"] = "3M"
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	facade.modelConfig = cfg

	conf := statushistorypruner.Config{
		Facade:        facade,
		PruneInterval: coretesting.ShortWait,
		NewTimer:      fakeTimerFunc,
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
	return facade, fakeTimer
}

func (s *statusHistoryPrunerSuite) assertWorkerCallsPrune(c *gc.C, facade *fakeFacade, fakeTimer *mockTimer, collectionSize int) {
	err := fakeTimer.fire()
	c.Check(err, jc.ErrorIsNil)

	var passedMB int
	select {
	case passedMB = <-facade.passedMaxHistoryMB:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for passed logs to pruner")
	}
	c.Assert(passedMB, gc.Equals, collectionSize)

	// Reset will have been called with the actual PruneInterval
	var period time.Duration
	select {
	case period = <-fakeTimer.period:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for period reset by pruner")
	}
	c.Assert(period, gc.Equals, coretesting.ShortWait)
}

func (s *statusHistoryPrunerSuite) TestWorkerCallsPrune(c *gc.C) {
	facade, fakeTimer := s.setupPruner(c)
	s.assertWorkerCallsPrune(c, facade, fakeTimer, 3)
}

func (s *statusHistoryPrunerSuite) TestWorkerWontCallPruneBeforeFiringTimer(c *gc.C) {
	facade, _ := s.setupPruner(c)

	select {
	case <-facade.passedMaxHistoryMB:
		c.Fatal("called before firing timer.")
	case <-time.After(coretesting.LongWait):
	}
}

func (s *statusHistoryPrunerSuite) TestModelConfigChange(c *gc.C) {
	facade, fakeTimer := s.setupPruner(c)
	s.assertWorkerCallsPrune(c, facade, fakeTimer, 3)

	var err error
	facade.modelConfig, err = facade.modelConfig.Apply(map[string]interface{}{"max-status-history-size": "4M"})
	c.Assert(err, jc.ErrorIsNil)
	facade.changesWatcher.changes <- struct{}{}

	s.assertWorkerCallsPrune(c, facade, fakeTimer, 4)
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
	passedMaxHistoryMB chan int
	changesWatcher     *mockNotifyWatcher
	modelConfig        *config.Config
	gotConfig          chan struct{}
}

func newFakeFacade() *fakeFacade {
	return &fakeFacade{
		passedMaxHistoryMB: make(chan int, 1),
		gotConfig:          make(chan struct{}, 1),
		changesWatcher:     newMockNotifyWatcher(),
	}
}

// Prune implements Facade
func (f *fakeFacade) Prune(_ time.Duration, maxHistoryMB int) error {
	// TODO(perrito666) either make this send its actual args, or just use
	// a stub and drop the unnecessary channel malarkey entirely
	select {
	case f.passedMaxHistoryMB <- maxHistoryMB:
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
