// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/provider/caas"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
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
		appChan <- []string{"test"}
		return watchertest.NewMockStringsWatcher(appChan), nil
	})
	facade.EXPECT().ProvisionerConfig().Return(params.CAASApplicationProvisionerConfig{
		UnmanagedApplications: params.Entities{},
	}, nil)
	facade.EXPECT().Life("test").Return(life.Alive, nil)
	runner.EXPECT().Worker("test", gomock.Any()).Return(nil, errors.NotFoundf(""))
	runner.EXPECT().StartWorker("test", gomock.Any()).DoAndReturn(
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
			Name:     "test",
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

func (s *CAASApplicationSuite) TestWorkerStartUnmanaged(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appChan := make(chan []string, 1)
	done := make(chan struct{})

	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	runner := mocks.NewMockRunner(ctrl)

	facade.EXPECT().WatchApplications().DoAndReturn(func() (watcher.StringsWatcher, error) {
		appChan <- []string{"test"}
		return watchertest.NewMockStringsWatcher(appChan), nil
	})
	facade.EXPECT().ProvisionerConfig().Return(params.CAASApplicationProvisionerConfig{
		UnmanagedApplications: params.Entities{Entities: []params.Entity{{Tag: "application-test"}}},
	}, nil)
	facade.EXPECT().Life("test").Return(life.Alive, nil)
	runner.EXPECT().Worker("test", gomock.Any()).Return(nil, errors.NotFoundf(""))
	runner.EXPECT().StartWorker("test", gomock.Any()).DoAndReturn(
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
			Name:       "test",
			ModelTag:   s.modelTag,
			StatusOnly: true,
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

func (s *CAASApplicationSuite) TestWorkerStartOnceNotify(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appChan := make(chan []string, 5)
	done := make(chan struct{})

	appChan <- []string{"test"}
	appChan <- []string{"test"}
	appChan <- []string{"test"}
	appChan <- []string{"test"}
	appChan <- []string{"test"}
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	runner := mocks.NewMockRunner(ctrl)

	var notifyWorker = &mockNotifyWorker{Worker: workertest.NewErrorWorker(nil)}

	gomock.InOrder(
		facade.EXPECT().WatchApplications().DoAndReturn(func() (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(appChan), nil
		}),
		facade.EXPECT().ProvisionerConfig().Return(params.CAASApplicationProvisionerConfig{}, nil),
		facade.EXPECT().Life("test").Return(life.Alive, nil),
		runner.EXPECT().Worker("test", gomock.Any()).Return(nil, errors.NotFoundf("")),
		runner.EXPECT().StartWorker("test", gomock.Any()).DoAndReturn(
			func(_ string, startFunc func() (worker.Worker, error)) error {
				startFunc()
				return nil
			},
		),

		facade.EXPECT().Life("test").Return(life.Alive, nil),
		runner.EXPECT().Worker("test", gomock.Any()).Return(notifyWorker, nil),

		facade.EXPECT().Life("test").Return(life.Alive, nil),
		runner.EXPECT().Worker("test", gomock.Any()).Return(notifyWorker, nil),

		facade.EXPECT().Life("test").Return(life.Alive, nil),
		runner.EXPECT().Worker("test", gomock.Any()).Return(notifyWorker, nil),

		facade.EXPECT().Life("test").Return(life.Alive, nil),
		runner.EXPECT().Worker("test", gomock.Any()).DoAndReturn(
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
			Name:     "test",
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
