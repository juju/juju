// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasapplicationprovisioner"
)

var _ = gc.Suite(&CAASApplicationSuite{})

type CAASApplicationSuite struct {
	coretesting.BaseSuite

	clock    *testclock.Clock
	modelTag names.ModelTag
	logger   loggo.Logger
}

func (s *CAASApplicationSuite) SetUpTest(c *gc.C) {
	s.clock = testclock.NewClock(time.Now())
	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
}

func (s *CAASApplicationSuite) TestWorkerStart(c *gc.C) {
	appChan := make(chan []string, 1)
	appChan <- []string{"application-test"}
	facade := &mockFacade{
		appWatcher: watchertest.NewMockStringsWatcher(appChan),
	}
	called := false
	workerChan := make(chan struct{})
	newWorker := func(config caasapplicationprovisioner.AppWorkerConfig) func() (worker.Worker, error) {
		c.Assert(called, jc.IsFalse)
		called = true
		mc := jc.NewMultiChecker()
		mc.AddExpr("_.Facade", gc.NotNil)
		mc.AddExpr("_.Broker", gc.NotNil)
		mc.AddExpr("_.Clock", gc.NotNil)
		mc.AddExpr("_.Logger", gc.NotNil)
		c.Check(config, mc, caasapplicationprovisioner.AppWorkerConfig{
			Name:     "application-test",
			ModelTag: s.modelTag,
		})
		return func() (worker.Worker, error) {
			close(workerChan)
			return workertest.NewErrorWorker(nil), nil
		}
	}
	config := caasapplicationprovisioner.Config{
		Facade:       facade,
		Broker:       struct{ caas.Broker }{},
		ModelTag:     s.modelTag,
		Clock:        s.clock,
		Logger:       s.logger,
		NewAppWorker: newWorker,
	}
	provisioner, err := caasapplicationprovisioner.NewProvisionerWorker(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioner, gc.NotNil)

	select {
	case <-workerChan:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	c.Assert(called, jc.IsTrue)
	workertest.CleanKill(c, provisioner)
}

func (s *CAASApplicationSuite) TestWorkerStartOnceNotify(c *gc.C) {
	appChan := make(chan []string, 4)
	appChan <- []string{"application-test"}
	appChan <- []string{"application-test"}
	appChan <- []string{"application-test"}
	appChan <- []string{"application-test"}
	facade := &mockFacade{
		appWatcher: watchertest.NewMockStringsWatcher(appChan),
	}
	called := 0
	workerChan := make(chan struct{})
	var notifyWorker = &mockNotifyWorker{Worker: workertest.NewErrorWorker(nil)}
	newWorker := func(config caasapplicationprovisioner.AppWorkerConfig) func() (worker.Worker, error) {
		called++
		mc := jc.NewMultiChecker()
		mc.AddExpr("_.Facade", gc.NotNil)
		mc.AddExpr("_.Broker", gc.NotNil)
		mc.AddExpr("_.Clock", gc.NotNil)
		mc.AddExpr("_.Logger", gc.NotNil)
		c.Check(config, mc, caasapplicationprovisioner.AppWorkerConfig{
			Name:     "application-test",
			ModelTag: s.modelTag,
		})
		return func() (worker.Worker, error) {
			close(workerChan)
			return notifyWorker, nil
		}
	}
	config := caasapplicationprovisioner.Config{
		Facade:       facade,
		Broker:       struct{ caas.Broker }{},
		ModelTag:     s.modelTag,
		Clock:        s.clock,
		Logger:       s.logger,
		NewAppWorker: newWorker,
	}
	provisioner, err := caasapplicationprovisioner.NewProvisionerWorker(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioner, gc.NotNil)

	select {
	case <-workerChan:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	c.Assert(called, gc.Equals, 1)
	c.Assert(notifyWorker, gc.NotNil)
	select {
	case <-time.After(coretesting.ShortWait):
		workertest.CleanKill(c, provisioner)
		notifyWorker.CheckCallNames(c, "Notify", "Notify", "Notify")
	}
}
