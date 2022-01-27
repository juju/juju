// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasapplicationprovisioner"
	"github.com/juju/juju/worker/caasapplicationprovisioner/mocks"
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appChan := make(chan []string, 1)
	done := make(chan struct{})

	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	runner := mocks.NewMockRunner(ctrl)

	facade.EXPECT().WatchApplications().DoAndReturn(func() (watcher.StringsWatcher, error) {
		appChan <- []string{"application-test"}
		return watchertest.NewMockStringsWatcher(appChan), nil
	})
	facade.EXPECT().Life("application-test").Return(life.Alive, nil)
	runner.EXPECT().Worker("application-test", gomock.Any()).Return(nil, errors.NotFoundf(""))
	runner.EXPECT().StartWorker("application-test", gomock.Any()).DoAndReturn(
		func(_ string, startFunc func() (worker.Worker, error)) error {
			startFunc()
			return nil
		},
	)
	runner.EXPECT().Wait().AnyTimes().DoAndReturn(func() error {
		<-done
		return nil
	})
	runner.EXPECT().Kill().AnyTimes()

	called := false
	newWorker := func(config caasapplicationprovisioner.AppWorkerConfig) func() (worker.Worker, error) {
		c.Assert(called, jc.IsFalse)
		called = true
		mc := jc.NewMultiChecker()
		mc.AddExpr("_.Facade", gc.NotNil)
		mc.AddExpr("_.Broker", gc.NotNil)
		mc.AddExpr("_.Clock", gc.NotNil)
		mc.AddExpr("_.Logger", gc.NotNil)
		mc.AddExpr("_.ShutDownCleanUpFunc", gc.NotNil)
		c.Check(config, mc, caasapplicationprovisioner.AppWorkerConfig{
			Name:     "application-test",
			ModelTag: s.modelTag,
		})
		return func() (worker.Worker, error) {
			close(done)
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
	provisioner, err := caasapplicationprovisioner.NewProvisionerWorkerForTest(config, runner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioner, gc.NotNil)

	select {
	case <-done:
		c.Assert(called, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, provisioner)
}

func (s *CAASApplicationSuite) TestWorkerShutdownForDeadApp(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appChan := make(chan []string, 1)
	done := make(chan struct{})

	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	runner := mocks.NewMockRunner(ctrl)

	facade.EXPECT().WatchApplications().DoAndReturn(func() (watcher.StringsWatcher, error) {
		appChan <- []string{"application-test"}
		return watchertest.NewMockStringsWatcher(appChan), nil
	})
	facade.EXPECT().Life("application-test").DoAndReturn(func(appName string) (life.Value, error) {
		return life.Dead, nil
	})
	runner.EXPECT().StopAndRemoveWorker("application-test", gomock.Any()).DoAndReturn(func(string, <-chan struct{}) error {
		close(done)
		return errors.NotFoundf("")
	})
	runner.EXPECT().Wait().AnyTimes().DoAndReturn(func() error {
		<-done
		return nil
	})
	runner.EXPECT().Kill().AnyTimes()

	newWorker := func(config caasapplicationprovisioner.AppWorkerConfig) func() (worker.Worker, error) {
		return func() (worker.Worker, error) {
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
	provisioner, err := caasapplicationprovisioner.NewProvisionerWorkerForTest(config, runner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioner, gc.NotNil)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, provisioner)
}

func (s *CAASApplicationSuite) TestWorkerStartOnceNotify(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appChan := make(chan []string, 5)
	done := make(chan struct{})

	appChan <- []string{"application-test"}
	appChan <- []string{"application-test"}
	appChan <- []string{"application-test"}
	appChan <- []string{"application-test"}
	appChan <- []string{"application-test"}
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	runner := mocks.NewMockRunner(ctrl)

	var notifyWorker = &mockNotifyWorker{Worker: workertest.NewErrorWorker(nil)}

	gomock.InOrder(
		facade.EXPECT().WatchApplications().DoAndReturn(func() (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(appChan), nil
		}),
		facade.EXPECT().Life("application-test").Return(life.Alive, nil),
		runner.EXPECT().Worker("application-test", gomock.Any()).Return(nil, errors.NotFoundf("")),
		runner.EXPECT().StartWorker("application-test", gomock.Any()).DoAndReturn(
			func(_ string, startFunc func() (worker.Worker, error)) error {
				startFunc()
				return nil
			},
		),

		facade.EXPECT().Life("application-test").Return(life.Alive, nil),
		runner.EXPECT().Worker("application-test", gomock.Any()).Return(notifyWorker, nil),

		facade.EXPECT().Life("application-test").Return(life.Alive, nil),
		runner.EXPECT().Worker("application-test", gomock.Any()).Return(notifyWorker, nil),

		facade.EXPECT().Life("application-test").Return(life.Alive, nil),
		runner.EXPECT().Worker("application-test", gomock.Any()).Return(notifyWorker, nil),

		facade.EXPECT().Life("application-test").Return(life.Alive, nil),
		runner.EXPECT().Worker("application-test", gomock.Any()).DoAndReturn(
			func(_ string, abort <-chan struct{}) (worker.Worker, error) {
				close(done)
				return nil, worker.ErrDead
			},
		),
	)
	runner.EXPECT().Wait().AnyTimes().DoAndReturn(func() error {
		<-done
		return nil
	})
	runner.EXPECT().Kill().AnyTimes()

	called := 0
	newWorker := func(config caasapplicationprovisioner.AppWorkerConfig) func() (worker.Worker, error) {
		called++
		mc := jc.NewMultiChecker()
		mc.AddExpr("_.Facade", gc.NotNil)
		mc.AddExpr("_.Broker", gc.NotNil)
		mc.AddExpr("_.Clock", gc.NotNil)
		mc.AddExpr("_.Logger", gc.NotNil)
		mc.AddExpr("_.ShutDownCleanUpFunc", gc.NotNil)
		c.Check(config, mc, caasapplicationprovisioner.AppWorkerConfig{
			Name:     "application-test",
			ModelTag: s.modelTag,
		})
		return func() (worker.Worker, error) {
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
	provisioner, err := caasapplicationprovisioner.NewProvisionerWorkerForTest(config, runner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provisioner, gc.NotNil)

	select {
	case <-done:
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
