// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&ApplicationWorkerSuite{})

type ApplicationWorkerSuite struct {
	coretesting.BaseSuite

	modelTag names.ModelTag
	logger   loggo.Logger
}

func (s *ApplicationWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
}

func (s *ApplicationWorkerSuite) waitDone(c *gc.C, done chan struct{}) {
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) startAppWorker(
	c *gc.C,
	clk clock.Clock,
	facade caasapplicationprovisioner.CAASProvisionerFacade,
	broker caasapplicationprovisioner.CAASBroker,
	unitFacade caasapplicationprovisioner.CAASUnitProvisionerFacade,
	ops caasapplicationprovisioner.ApplicationOps,
	statusOnly bool,
) worker.Worker {
	config := caasapplicationprovisioner.AppWorkerConfig{
		Name:       "test",
		Facade:     facade,
		Broker:     broker,
		ModelTag:   s.modelTag,
		Clock:      clk,
		Logger:     s.logger,
		UnitFacade: unitFacade,
		Ops:        ops,
		StatusOnly: statusOnly,
	}
	startFunc := caasapplicationprovisioner.NewAppWorker(config)
	c.Assert(startFunc, gc.NotNil)
	appWorker, err := startFunc()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appWorker, gc.NotNil)
	return appWorker
}

func (s *ApplicationWorkerSuite) TestLifeNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	brokerApp := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			close(done)
			return "", errors.NotFoundf("test charm")
		}),
	)
	appWorker := s.startAppWorker(c, nil, facade, broker, nil, ops, false)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestLifeDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	clk := testclock.NewDilatedWallClock(time.Millisecond)

	done := make(chan struct{})

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		facade.EXPECT().Life("test").Return(life.Dead, nil),
		ops.EXPECT().AppDying("test", app, life.Dead, facade, unitFacade, s.logger).Return(nil),
		ops.EXPECT().AppDead("test", app, broker, facade, unitFacade, clk, s.logger).DoAndReturn(func(_, _, _, _, _, _, _ any) error {
			close(done)
			return nil
		}),
	)
	appWorker := s.startAppWorker(c, clk, facade, broker, unitFacade, ops, false)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestWorker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	done := make(chan struct{})

	clk := testclock.NewDilatedWallClock(time.Millisecond)

	scaleChan := make(chan struct{}, 1)
	trustChan := make(chan []string, 1)
	provisioningInfoChan := make(chan struct{}, 1)
	appUnitsChan := make(chan []string, 1)
	appChan := make(chan struct{}, 1)
	appReplicasChan := make(chan struct{}, 1)

	ops.EXPECT().RefreshApplicationStatus("test", app, gomock.Any(), facade, s.logger).Return(nil).AnyTimes()

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		facade.EXPECT().Life("test").Return(life.Alive, nil),

		ops.EXPECT().CheckCharmFormat("test", gomock.Any(), gomock.Any()).Return(true, nil),

		facade.EXPECT().SetPassword("test", gomock.Any()).Return(nil),

		unitFacade.EXPECT().WatchApplicationScale("test").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		unitFacade.EXPECT().WatchApplicationTrustHash("test").Return(watchertest.NewMockStringsWatcher(trustChan), nil),
		facade.EXPECT().WatchUnits("test").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		facade.EXPECT().Life("test").Return(life.Alive, nil),
		facade.EXPECT().ProvisioningState("test").Return(nil, nil),
		facade.EXPECT().WatchProvisioningInfo("test").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		ops.EXPECT().AppAlive("test", app, gomock.Any(), gomock.Any(), facade, clk, s.logger).Return(nil),
		app.EXPECT().Watch().Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			scaleChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// scaleChan fired
		ops.EXPECT().EnsureScale("test", app, life.Alive, facade, unitFacade, s.logger).Return(errors.NotFound),
		ops.EXPECT().EnsureScale("test", app, life.Alive, facade, unitFacade, s.logger).Return(errors.ConstError("try again")),
		ops.EXPECT().EnsureScale("test", app, life.Alive, facade, unitFacade, s.logger).DoAndReturn(func(_, _, _, _, _, _ any) error {
			trustChan <- nil
			return nil
		}),

		// trustChan fired
		ops.EXPECT().EnsureTrust("test", app, unitFacade, s.logger).Return(errors.NotFound),
		ops.EXPECT().EnsureTrust("test", app, unitFacade, s.logger).DoAndReturn(func(_, _, _, _ any) error {
			appUnitsChan <- nil
			return nil
		}),

		// appUnitsChan fired
		ops.EXPECT().ReconcileDeadUnitScale("test", app, facade, s.logger).Return(errors.NotFound),
		ops.EXPECT().ReconcileDeadUnitScale("test", app, facade, s.logger).Return(errors.ConstError("try again")),
		ops.EXPECT().ReconcileDeadUnitScale("test", app, facade, s.logger).DoAndReturn(func(_, _, _, _ any) error {
			appChan <- struct{}{}
			return nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState("test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_, _, _, _, _, _, _ any) (map[string]status.StatusInfo, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState("test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_, _, _, _, _, _, _ any) (map[string]status.StatusInfo, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		facade.EXPECT().Life("test").Return(life.Alive, nil),
		ops.EXPECT().AppAlive("test", app, gomock.Any(), gomock.Any(), facade, clk, s.logger).DoAndReturn(func(_, _, _, _, _, _, _ any) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		facade.EXPECT().Life("test").Return(life.Dying, nil),
		ops.EXPECT().AppDying("test", app, life.Dying, facade, unitFacade, s.logger).DoAndReturn(func(_, _, _, _, _, _ any) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		facade.EXPECT().Life("test").Return(life.Dead, nil),
		ops.EXPECT().AppDying("test", app, life.Dead, facade, unitFacade, s.logger).Return(nil),
		ops.EXPECT().AppDead("test", app, broker, facade, unitFacade, clk, s.logger).DoAndReturn(func(_, _, _, _, _, _, _ any) error {
			close(done)
			return nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, unitFacade, ops, false)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestWorkerStatusOnly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	done := make(chan struct{})

	clk := testclock.NewDilatedWallClock(time.Millisecond)

	scaleChan := make(chan struct{}, 1)
	trustChan := make(chan []string, 1)
	provisioningInfoChan := make(chan struct{}, 1)
	appUnitsChan := make(chan []string, 1)
	appChan := make(chan struct{}, 1)
	appReplicasChan := make(chan struct{}, 1)

	ops.EXPECT().RefreshApplicationStatus("test", app, gomock.Any(), facade, s.logger).Return(nil).AnyTimes()

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		facade.EXPECT().Life("test").Return(life.Alive, nil),

		ops.EXPECT().CheckCharmFormat("test", gomock.Any(), gomock.Any()).Return(true, nil),

		unitFacade.EXPECT().WatchApplicationScale("test").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		unitFacade.EXPECT().WatchApplicationTrustHash("test").Return(watchertest.NewMockStringsWatcher(trustChan), nil),
		facade.EXPECT().WatchUnits("test").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		facade.EXPECT().Life("test").Return(life.Alive, nil),
		facade.EXPECT().ProvisioningState("test").Return(&params.CAASApplicationProvisioningState{Scaling: true, ScaleTarget: 1}, nil),
		facade.EXPECT().SetProvisioningState("test", params.CAASApplicationProvisioningState{}).Return(nil),
		facade.EXPECT().WatchProvisioningInfo("test").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		app.EXPECT().Watch().Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			appChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState("test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_, _, _, _, _, _, _ any) (map[string]status.StatusInfo, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState("test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_, _, _, _, _, _, _ any) (map[string]status.StatusInfo, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		facade.EXPECT().Life("test").DoAndReturn(func(_ string) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			return life.Alive, nil
		}),
		facade.EXPECT().Life("test").DoAndReturn(func(_ string) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			return life.Dying, nil
		}),
		facade.EXPECT().Life("test").DoAndReturn(func(_ string) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			close(done)
			return life.Dead, nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, unitFacade, ops, true)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}
