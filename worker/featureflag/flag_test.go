// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featureflag_test

import (
	"sync"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/featureflag"
)

type WorkerSuite struct {
	testing.IsolationSuite
	config  featureflag.Config
	source  *configSource
	changes chan struct{}
	worker  worker.Worker
	flag    engine.Flag
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.changes = make(chan struct{})
	s.source = &configSource{
		cfg: controller.Config{
			"features": []interface{}{"new-hotness"},
		},
		watcher: watchertest.NewNotifyWatcher(s.changes),
	}
	s.config = featureflag.Config{
		Source:   s.source,
		FlagName: "new-hotness",
	}
	worker, err := featureflag.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
	s.flag = worker.(engine.Flag)
}

func (s *WorkerSuite) TestCleanKill(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestCheckFeatureFlag(c *gc.C) {
	c.Assert(s.flag.Check(), jc.IsTrue)
}

func (s *WorkerSuite) TestCheckInverted(c *gc.C) {
	s.config.Invert = true
	w, err := featureflag.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, w)
	})

	c.Assert(w.(engine.Flag).Check(), jc.IsFalse)
}

func (s *WorkerSuite) TestFlagOff(c *gc.C) {
	s.source.setConfig(controller.Config{"features": []interface{}{}})
	w, err := featureflag.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, w)
	})

	c.Assert(w.(engine.Flag).Check(), jc.IsFalse)
}

func (s *WorkerSuite) TestDiesWhenFlagChanges(c *gc.C) {
	s.source.setConfig(controller.Config{"features": []interface{}{}})
	select {
	case s.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out sending config change")
	}

	c.Assert(workertest.CheckKilled(c, s.worker), gc.Equals, featureflag.ErrRefresh)
}

func (s *WorkerSuite) TestNoDieWhenNoChange(c *gc.C) {
	select {
	case s.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out sending config change")
	}
	time.Sleep(coretesting.ShortWait)
	workertest.CheckAlive(c, s.worker)
}

type configSource struct {
	mu      sync.Mutex
	stub    testing.Stub
	watcher *watchertest.NotifyWatcher
	cfg     controller.Config
}

func (s *configSource) WatchControllerConfig() state.NotifyWatcher {
	s.stub.AddCall("WatchControllerConfig")
	return s.watcher
}

func (s *configSource) ControllerConfig() (controller.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stub.AddCall("ControllerConfig")
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.cfg, nil
}

func (s *configSource) setConfig(cfg controller.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
}
