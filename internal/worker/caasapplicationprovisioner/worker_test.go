// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&CAASApplicationSuite{})

type CAASApplicationSuite struct {
	coretesting.BaseSuite

	clock    *testclock.Clock
	modelTag names.ModelTag
	logger   logger.Logger
}

func (s *CAASApplicationSuite) SetUpTest(c *tc.C) {
	s.clock = testclock.NewClock(time.Now())
	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggertesting.WrapCheckLog(c)
}

func (s *CAASApplicationSuite) TestWorkerStart(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appChan := make(chan []string, 1)
	done := make(chan struct{})

	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	runner := mocks.NewMockRunner(ctrl)

	facade.EXPECT().WatchApplications(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		appChan <- []string{"test"}
		return watchertest.NewMockStringsWatcher(appChan), nil
	})
	facade.EXPECT().ProvisionerConfig(gomock.Any()).Return(params.CAASApplicationProvisionerConfig{
		UnmanagedApplications: params.Entities{},
	}, nil)
	facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil)
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
		c.Assert(called, tc.IsFalse)
		called = true
		mc := tc.NewMultiChecker()
		mc.AddExpr("_.Facade", tc.NotNil)
		mc.AddExpr("_.Broker", tc.NotNil)
		mc.AddExpr("_.Clock", tc.NotNil)
		mc.AddExpr("_.Logger", tc.NotNil)
		mc.AddExpr("_.ShutDownCleanUpFunc", tc.NotNil)
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provisioner, tc.NotNil)

	select {
	case <-done:
		c.Assert(called, tc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, provisioner)
}

func (s *CAASApplicationSuite) TestWorkerStartUnmanaged(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appChan := make(chan []string, 1)
	done := make(chan struct{})

	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	runner := mocks.NewMockRunner(ctrl)

	facade.EXPECT().WatchApplications(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
		appChan <- []string{"test"}
		return watchertest.NewMockStringsWatcher(appChan), nil
	})
	facade.EXPECT().ProvisionerConfig(gomock.Any()).Return(params.CAASApplicationProvisionerConfig{
		UnmanagedApplications: params.Entities{Entities: []params.Entity{{Tag: "application-test"}}},
	}, nil)
	facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil)
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
		c.Assert(called, tc.IsFalse)
		called = true
		mc := tc.NewMultiChecker()
		mc.AddExpr("_.Facade", tc.NotNil)
		mc.AddExpr("_.Broker", tc.NotNil)
		mc.AddExpr("_.Clock", tc.NotNil)
		mc.AddExpr("_.Logger", tc.NotNil)
		mc.AddExpr("_.ShutDownCleanUpFunc", tc.NotNil)
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provisioner, tc.NotNil)

	select {
	case <-done:
		c.Assert(called, tc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, provisioner)
}

func (s *CAASApplicationSuite) TestWorkerStartOnceNotify(c *tc.C) {
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
		facade.EXPECT().WatchApplications(gomock.Any()).DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(appChan), nil
		}),
		facade.EXPECT().ProvisionerConfig(gomock.Any()).Return(params.CAASApplicationProvisionerConfig{}, nil),
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		runner.EXPECT().Worker("test", gomock.Any()).Return(nil, errors.NotFoundf("")),
		runner.EXPECT().StartWorker("test", gomock.Any()).DoAndReturn(
			func(_ string, startFunc func() (worker.Worker, error)) error {
				startFunc()
				return nil
			},
		),

		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		runner.EXPECT().Worker("test", gomock.Any()).Return(notifyWorker, nil),

		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		runner.EXPECT().Worker("test", gomock.Any()).Return(notifyWorker, nil),

		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		runner.EXPECT().Worker("test", gomock.Any()).Return(notifyWorker, nil),

		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
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
		mc := tc.NewMultiChecker()
		mc.AddExpr("_.Facade", tc.NotNil)
		mc.AddExpr("_.Broker", tc.NotNil)
		mc.AddExpr("_.Clock", tc.NotNil)
		mc.AddExpr("_.Logger", tc.NotNil)
		mc.AddExpr("_.ShutDownCleanUpFunc", tc.NotNil)
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provisioner, tc.NotNil)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	c.Assert(called, tc.Equals, 1)
	c.Assert(notifyWorker, tc.NotNil)
	select {
	case <-time.After(coretesting.ShortWait):
		workertest.CleanKill(c, provisioner)
		notifyWorker.CheckCallNames(c, "Notify", "Notify", "Notify")
	}
}
