// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pruner_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/pruner"
	"github.com/juju/juju/internal/worker/pruner/mocks"
	"github.com/juju/juju/internal/worker/statushistorypruner"
	coretesting "github.com/juju/juju/testing"
)

type PrunerSuite struct {
	coretesting.BaseSuite

	modelConfigService *mocks.MockModelConfigService
}

var _ = gc.Suite(&PrunerSuite{})

type newPrunerFunc func(pruner.Config) (worker.Worker, error)

func (s *PrunerSuite) TestWorkerCallsPrune(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	watcher := watchertest.NewMockStringsWatcher(ch)

	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(s.getConfig(c, "3M"), nil)

	// Send the initial event, followed by another.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ch <- []string{}
		ch <- []string{}
	}()

	pruner, facade, clock := s.newPruner(c, statushistorypruner.New)
	defer workertest.CleanKill(c, pruner)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for initial event")
	}

	s.assertWorkerCallsPrune(c, facade, clock, 3)
}

func (s *PrunerSuite) TestModelConfigChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	watcher := watchertest.NewMockStringsWatcher(ch)

	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(s.getConfig(c, "3M"), nil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(s.getConfig(c, "4M"), nil)

	// Send the initial event, followed by another.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ch <- []string{}
		ch <- []string{}
		ch <- []string{}
	}()

	pruner, facade, clock := s.newPruner(c, statushistorypruner.New)
	defer workertest.CleanKill(c, pruner)

	// Ensure we're done sending.
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for initial event")
	}

	s.assertWorkerCallsPrune(c, facade, clock, 4)
}

func (s *PrunerSuite) TestWorkerWontCallPruneBeforeFiringTimer(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	watcher := watchertest.NewMockStringsWatcher(ch)

	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)

	ch <- []string{}

	pruner, facade, _ := s.newPruner(c, statushistorypruner.New)
	defer workertest.CleanKill(c, pruner)

	select {
	case <-facade.pruned:
		c.Fatal("called before firing timer.")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *PrunerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelConfigService = mocks.NewMockModelConfigService(ctrl)

	return ctrl
}

func (s *PrunerSuite) newPruner(c *gc.C, newPruner newPrunerFunc) (worker.Worker, *fakeFacade, *testclock.Clock) {
	facade := newFakeFacade()

	testClock := testclock.NewClock(time.Time{})
	conf := pruner.Config{
		Facade:             facade,
		ModelConfigService: s.modelConfigService,
		PruneInterval:      coretesting.ShortWait,
		Clock:              testClock,
		Logger:             loggertesting.WrapCheckLog(c),
	}

	pruner, err := newPruner(conf)
	c.Check(err, jc.ErrorIsNil)

	return pruner, facade, testClock
}

func (s *PrunerSuite) assertWorkerCallsPrune(c *gc.C, facade *fakeFacade, testClock *testclock.Clock, collectionSize int) {
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

func (s *PrunerSuite) getConfig(c *gc.C, size string) *config.Config {
	attrs := coretesting.FakeConfig()
	attrs["max-status-history-age"] = "1s"
	attrs["max-status-history-size"] = size

	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

type fakeFacade struct {
	pruned chan pruneParams
}

type pruneParams struct {
	maxAge       time.Duration
	maxHistoryMB int
}

func newFakeFacade() *fakeFacade {
	return &fakeFacade{
		pruned: make(chan pruneParams, 1),
	}
}

// Prune implements Facade
func (f *fakeFacade) Prune(maxAge time.Duration, maxHistoryMB int) error {
	select {
	case f.pruned <- pruneParams{maxAge: maxAge, maxHistoryMB: maxHistoryMB}:
	case <-time.After(coretesting.LongWait):
		return errors.New("timed out waiting for facade call Prune to run")
	}
	return nil
}
