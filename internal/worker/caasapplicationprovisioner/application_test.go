// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&ApplicationWorkerSuite{})

type ApplicationWorkerSuite struct {
	coretesting.BaseSuite

	modelTag names.ModelTag
	logger   logger.Logger
}

func (s *ApplicationWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggertesting.WrapCheckLog(c)
}

func (s *ApplicationWorkerSuite) waitDone(c *gc.C, done chan struct{}) {
	select {
	case <-done:
	case <-time.After(1000 * coretesting.LongWait):
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
	appWorker, err := startFunc(context.Background())
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
		facade.EXPECT().Life(gomock.Any(), "test").DoAndReturn(func(ctx context.Context, appName string) (life.Value, error) {
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
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Dead, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", app, life.Dead, facade, unitFacade, s.logger).Return(nil),
		ops.EXPECT().AppDead(gomock.Any(), "test", app, broker, facade, unitFacade, clk, s.logger).
			DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ caasapplicationprovisioner.CAASBroker,
				_ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade,
				_ clock.Clock, _ logger.Logger) error {
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

	ops.EXPECT().RefreshApplicationStatus(gomock.Any(), "test", app, gomock.Any(), facade, s.logger).Return(nil).AnyTimes()

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),

		ops.EXPECT().CheckCharmFormat(gomock.Any(), "test", gomock.Any(), gomock.Any()).Return(true, nil),

		facade.EXPECT().SetPassword(gomock.Any(), "test", gomock.Any()).Return(nil),

		unitFacade.EXPECT().WatchApplicationScale(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		unitFacade.EXPECT().WatchApplicationTrustHash(gomock.Any(), "test").Return(watchertest.NewMockStringsWatcher(trustChan), nil),
		facade.EXPECT().WatchUnits(gomock.Any(), "test").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		facade.EXPECT().ProvisioningState(gomock.Any(), "test").Return(nil, nil),
		facade.EXPECT().WatchProvisioningInfo(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), facade, clk, s.logger).Return(nil),
		app.EXPECT().Watch(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			scaleChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// scaleChan fired
		ops.EXPECT().EnsureScale(gomock.Any(), "test", app, life.Alive, facade, unitFacade, s.logger).Return(errors.NotFound),
		ops.EXPECT().EnsureScale(gomock.Any(), "test", app, life.Alive, facade, unitFacade, s.logger).Return(errors.ConstError("try again")),
		ops.EXPECT().EnsureScale(gomock.Any(), "test", app, life.Alive, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, v life.Value, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) error {
			trustChan <- nil
			return nil
		}),

		// trustChan fired
		ops.EXPECT().EnsureTrust(gomock.Any(), "test", app, unitFacade, s.logger).Return(errors.NotFound),
		ops.EXPECT().EnsureTrust(gomock.Any(), "test", app, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) error {
			appUnitsChan <- nil
			return nil
		}),

		// appUnitsChan fired
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", app, facade, s.logger).Return(errors.NotFound),
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", app, facade, s.logger).Return(errors.ConstError("try again")),
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", app, facade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ caasapplicationprovisioner.CAASProvisionerFacade, _ logger.Logger) error {
			appChan <- struct{}{}
			return nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), facade, clk, s.logger).DoAndReturn(func(_ context.Context, s1 string, _ caas.Application, s2 string, ac *caas.ApplicationConfig, _ caasapplicationprovisioner.CAASProvisionerFacade, c clock.Clock, _ logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Dying, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", app, life.Dying, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, v life.Value, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Dead, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", app, life.Dead, facade, unitFacade, s.logger).Return(nil),
		ops.EXPECT().AppDead(gomock.Any(), "test", app, broker, facade, unitFacade, clk, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, c1 caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, c2 clock.Clock, _ logger.Logger) error {
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

	ops.EXPECT().RefreshApplicationStatus(gomock.Any(), "test", app, gomock.Any(), facade, s.logger).Return(nil).AnyTimes()

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),

		ops.EXPECT().CheckCharmFormat(gomock.Any(), "test", gomock.Any(), gomock.Any()).Return(true, nil),

		unitFacade.EXPECT().WatchApplicationScale(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		unitFacade.EXPECT().WatchApplicationTrustHash(gomock.Any(), "test").Return(watchertest.NewMockStringsWatcher(trustChan), nil),
		facade.EXPECT().WatchUnits(gomock.Any(), "test").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		facade.EXPECT().ProvisioningState(gomock.Any(), "test").Return(&params.CAASApplicationProvisioningState{Scaling: true, ScaleTarget: 1}, nil),
		facade.EXPECT().SetProvisioningState(gomock.Any(), "test", params.CAASApplicationProvisioningState{}).Return(nil),
		facade.EXPECT().WatchProvisioningInfo(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		app.EXPECT().Watch(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			appChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		facade.EXPECT().Life(gomock.Any(), "test").DoAndReturn(func(_ context.Context, _ string) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			return life.Alive, nil
		}),
		facade.EXPECT().Life(gomock.Any(), "test").DoAndReturn(func(_ context.Context, _ string) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			return life.Dying, nil
		}),
		facade.EXPECT().Life(gomock.Any(), "test").DoAndReturn(func(_ context.Context, _ string) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			close(done)
			return life.Dead, nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, unitFacade, ops, true)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestNotProvisionedRetry(c *gc.C) {
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

	ops.EXPECT().RefreshApplicationStatus(gomock.Any(), "test", app, gomock.Any(), facade, s.logger).Return(nil).AnyTimes()

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),

		ops.EXPECT().CheckCharmFormat(gomock.Any(), "test", gomock.Any(), gomock.Any()).Return(true, nil),

		facade.EXPECT().SetPassword(gomock.Any(), "test", gomock.Any()).Return(nil),

		unitFacade.EXPECT().WatchApplicationScale(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		unitFacade.EXPECT().WatchApplicationTrustHash(gomock.Any(), "test").Return(watchertest.NewMockStringsWatcher(trustChan), nil),
		facade.EXPECT().WatchUnits(gomock.Any(), "test").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		facade.EXPECT().ProvisioningState(gomock.Any(), "test").Return(nil, nil),
		facade.EXPECT().WatchProvisioningInfo(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		// error with not provisioned
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), facade, clk, s.logger).Return(errors.NotProvisioned),

		// retry handleChange
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), facade, clk, s.logger).Return(nil),
		app.EXPECT().Watch(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			scaleChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// scaleChan fired
		ops.EXPECT().EnsureScale(gomock.Any(), "test", app, life.Alive, facade, unitFacade, s.logger).Return(errors.NotFound),
		ops.EXPECT().EnsureScale(gomock.Any(), "test", app, life.Alive, facade, unitFacade, s.logger).Return(errors.ConstError("try again")),
		ops.EXPECT().EnsureScale(gomock.Any(), "test", app, life.Alive, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, v life.Value, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) error {
			trustChan <- nil
			return nil
		}),

		// trustChan fired
		ops.EXPECT().EnsureTrust(gomock.Any(), "test", app, unitFacade, s.logger).Return(errors.NotFound),
		ops.EXPECT().EnsureTrust(gomock.Any(), "test", app, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) error {
			appUnitsChan <- nil
			return nil
		}),

		// appUnitsChan fired
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", app, facade, s.logger).Return(errors.NotFound),
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", app, facade, s.logger).Return(errors.ConstError("try again")),
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", app, facade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ caasapplicationprovisioner.CAASProvisionerFacade, _ logger.Logger) error {
			appChan <- struct{}{}
			return nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "test", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Alive, nil),
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), facade, clk, s.logger).DoAndReturn(func(_ context.Context, s1 string, _ caas.Application, s2 string, ac *caas.ApplicationConfig, _ caasapplicationprovisioner.CAASProvisionerFacade, c clock.Clock, _ logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Dying, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", app, life.Dying, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, v life.Value, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		facade.EXPECT().Life(gomock.Any(), "test").Return(life.Dead, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", app, life.Dead, facade, unitFacade, s.logger).Return(nil),
		ops.EXPECT().AppDead(gomock.Any(), "test", app, broker, facade, unitFacade, clk, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, c1 caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, c2 clock.Clock, _ logger.Logger) error {
			close(done)
			return nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, unitFacade, ops, false)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}
