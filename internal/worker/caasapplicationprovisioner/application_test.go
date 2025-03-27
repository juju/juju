// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
)

func TestApplicationWorkerSuite(t *testing.T) {
	tc.Run(t, &ApplicationWorkerSuite{})
}

type ApplicationWorkerSuite struct {
	coretesting.BaseSuite

	appID    application.ID
	modelTag names.ModelTag
	logger   logger.Logger
}

func (s *ApplicationWorkerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.appID, err = application.NewID()
	c.Assert(err, tc.ErrorIsNil)

	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggertesting.WrapCheckLog(c)
}

func (s *ApplicationWorkerSuite) waitDone(c *tc.C, done chan struct{}) {
	select {
	case <-done:
	case <-time.After(1000 * coretesting.LongWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) startAppWorker(
	c *tc.C,
	clk clock.Clock,
	facade caasapplicationprovisioner.CAASProvisionerFacade,
	broker caasapplicationprovisioner.CAASBroker,
	unitFacade caasapplicationprovisioner.CAASUnitProvisionerFacade,
	ops caasapplicationprovisioner.ApplicationOps,
	applicationService caasapplicationprovisioner.ApplicationService,
	statusService caasapplicationprovisioner.StatusService,
) worker.Worker {
	config := caasapplicationprovisioner.AppWorkerConfig{
		AppID:              s.appID,
		Facade:             facade,
		Broker:             broker,
		ModelTag:           s.modelTag,
		Clock:              clk,
		Logger:             s.logger,
		UnitFacade:         unitFacade,
		Ops:                ops,
		ApplicationService: applicationService,
		StatusService:      statusService,
	}
	startFunc := caasapplicationprovisioner.NewAppWorker(config)
	c.Assert(startFunc, tc.NotNil)
	appWorker, err := startFunc(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appWorker, tc.NotNil)
	return appWorker
}

func (s *ApplicationWorkerSuite) TestLifeNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(gomock.Any(), s.appID).DoAndReturn(func(ctx context.Context, appID application.ID) (string, error) {
			close(done)
			return "", applicationerrors.ApplicationNotFound
		}),
	)
	appWorker := s.startAppWorker(c, nil, facade, broker, nil, ops, applicationService, statusService)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestLifeDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	clk := testclock.NewDilatedWallClock(time.Millisecond)

	done := make(chan struct{})

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(gomock.Any(), s.appID).Return("test", nil),
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Dead, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", s.appID, app, life.Dead, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		ops.EXPECT().AppDead(gomock.Any(), "test", app, broker, facade, unitFacade, clk, s.logger).
			DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ caasapplicationprovisioner.CAASBroker,
				_ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade,
				_ clock.Clock, _ logger.Logger) error {
				close(done)
				return nil
			}),
	)
	appWorker := s.startAppWorker(c, clk, facade, broker, unitFacade, ops, applicationService, statusService)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestWorker(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	done := make(chan struct{})

	clk := testclock.NewDilatedWallClock(time.Millisecond)

	scaleChan := make(chan struct{}, 1)
	settingsChan := make(chan struct{}, 1)
	provisioningInfoChan := make(chan struct{}, 1)
	appUnitsChan := make(chan []string, 1)
	appChan := make(chan struct{}, 1)
	appReplicasChan := make(chan struct{}, 1)

	ops.EXPECT().RefreshApplicationStatus(gomock.Any(), "test", s.appID, app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(gomock.Any(), s.appID).Return("test", nil),
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Alive, nil),

		ops.EXPECT().CheckCharmFormat(gomock.Any(), "test", gomock.Any(), gomock.Any()).Return(true, nil),

		facade.EXPECT().SetPassword(gomock.Any(), "test", gomock.Any()).Return(nil),

		unitFacade.EXPECT().WatchApplicationScale(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		applicationService.EXPECT().WatchApplicationSettings(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(settingsChan), nil),
		facade.EXPECT().WatchUnits(gomock.Any(), "test").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Alive, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		facade.EXPECT().WatchProvisioningInfo(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		app.EXPECT().Watch(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			scaleChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// scaleChan fired
		ops.EXPECT().EnsureScale(gomock.Any(), "test", s.appID, app, life.Alive, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.NotFound),
		ops.EXPECT().EnsureScale(gomock.Any(), "test", s.appID, app, life.Alive, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.ConstError("try again")),
		ops.EXPECT().EnsureScale(gomock.Any(), "test", s.appID, app, life.Alive, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, v life.Value, cf caasapplicationprovisioner.CAASProvisionerFacade, cpf caasapplicationprovisioner.CAASUnitProvisionerFacade, as caasapplicationprovisioner.ApplicationService, ss caasapplicationprovisioner.StatusService, l logger.Logger) error {
			settingsChan <- struct{}{}
			return nil
		}),

		// trustChan fired
		ops.EXPECT().EnsureTrust(gomock.Any(), "test", app, gomock.Any(), gomock.Any()).Return(errors.NotFound),
		ops.EXPECT().EnsureTrust(gomock.Any(), "test", app, gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s string, a caas.Application, as caasapplicationprovisioner.ApplicationService, l logger.Logger) error {
			appUnitsChan <- nil
			return nil
		}),

		// appUnitsChan fired
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", s.appID, app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.NotFound),
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", s.appID, app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.ConstError("try again")),
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", s.appID, app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, cf caasapplicationprovisioner.CAASProvisionerFacade, as caasapplicationprovisioner.ApplicationService, ss caasapplicationprovisioner.StatusService, l logger.Logger) error {
			appChan <- struct{}{}
			return nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Alive, nil),
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s1 string, a caas.Application, s2 string, ac *caas.ApplicationConfig, cf caasapplicationprovisioner.CAASProvisionerFacade, ss caasapplicationprovisioner.StatusService, c clock.Clock, l logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Dying, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", s.appID, app, life.Dying, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, v life.Value, cf caasapplicationprovisioner.CAASProvisionerFacade, cpf caasapplicationprovisioner.CAASUnitProvisionerFacade, as caasapplicationprovisioner.ApplicationService, ss caasapplicationprovisioner.StatusService, l logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Dead, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", s.appID, app, life.Dead, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		ops.EXPECT().AppDead(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s string, a caas.Application, c1 caasapplicationprovisioner.CAASBroker, cf caasapplicationprovisioner.CAASProvisionerFacade, cpf caasapplicationprovisioner.CAASUnitProvisionerFacade, c2 clock.Clock, l logger.Logger) error {
			close(done)
			return nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, unitFacade, ops, applicationService, statusService)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestWorkerStatusOnly(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	done := make(chan struct{})

	clk := testclock.NewDilatedWallClock(time.Millisecond)

	scaleChan := make(chan struct{}, 1)
	settingsChan := make(chan struct{}, 1)
	provisioningInfoChan := make(chan struct{}, 1)
	appUnitsChan := make(chan []string, 1)
	appChan := make(chan struct{}, 1)
	appReplicasChan := make(chan struct{}, 1)

	ops.EXPECT().RefreshApplicationStatus(gomock.Any(), "controller", s.appID, app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(gomock.Any(), s.appID).Return("controller", nil),
		broker.EXPECT().Application("controller", caas.DeploymentStateful).Return(app),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Alive, nil),

		ops.EXPECT().CheckCharmFormat(gomock.Any(), "controller", gomock.Any(), gomock.Any()).Return(true, nil),

		unitFacade.EXPECT().WatchApplicationScale(gomock.Any(), "controller").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		applicationService.EXPECT().WatchApplicationSettings(gomock.Any(), "controller").Return(watchertest.NewMockNotifyWatcher(settingsChan), nil),
		facade.EXPECT().WatchUnits(gomock.Any(), "controller").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Alive, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "controller").Return(applicationservice.ScalingState{Scaling: true, ScaleTarget: 1}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "controller", 0, false).Return(nil),
		facade.EXPECT().WatchProvisioningInfo(gomock.Any(), "controller").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		app.EXPECT().Watch(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			appChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "controller", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState(gomock.Any(), "controller", app, gomock.Any(), broker, facade, unitFacade, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ map[string]status.StatusInfo, _ caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, _ logger.Logger) (map[string]status.StatusInfo, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).DoAndReturn(func(ctx context.Context, i application.ID) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			return life.Alive, nil
		}),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).DoAndReturn(func(ctx context.Context, i application.ID) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			return life.Dying, nil
		}),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).DoAndReturn(func(ctx context.Context, i application.ID) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			close(done)
			return life.Dead, nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, unitFacade, ops, applicationService, statusService)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestNotProvisionedRetry(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	done := make(chan struct{})

	clk := testclock.NewDilatedWallClock(time.Millisecond)

	scaleChan := make(chan struct{}, 1)
	settingsChan := make(chan struct{}, 1)
	provisioningInfoChan := make(chan struct{}, 1)
	appUnitsChan := make(chan []string, 1)
	appChan := make(chan struct{}, 1)
	appReplicasChan := make(chan struct{}, 1)

	ops.EXPECT().RefreshApplicationStatus(gomock.Any(), "test", s.appID, app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(gomock.Any(), s.appID).Return("test", nil),
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Alive, nil),

		ops.EXPECT().CheckCharmFormat(gomock.Any(), "test", gomock.Any(), gomock.Any()).Return(true, nil),

		facade.EXPECT().SetPassword(gomock.Any(), "test", gomock.Any()).Return(nil),

		unitFacade.EXPECT().WatchApplicationScale(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		applicationService.EXPECT().WatchApplicationSettings(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(settingsChan), nil),
		facade.EXPECT().WatchUnits(gomock.Any(), "test").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Alive, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		facade.EXPECT().WatchProvisioningInfo(gomock.Any(), "test").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		// error with not provisioned
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.NotProvisioned),

		// retry handleChange
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Alive, nil),
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		app.EXPECT().Watch(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			scaleChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// scaleChan fired
		ops.EXPECT().EnsureScale(gomock.Any(), "test", s.appID, app, life.Alive, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.NotFound),
		ops.EXPECT().EnsureScale(gomock.Any(), "test", s.appID, app, life.Alive, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.ConstError("try again")),
		ops.EXPECT().EnsureScale(gomock.Any(), "test", s.appID, app, life.Alive, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, v life.Value, cf caasapplicationprovisioner.CAASProvisionerFacade, cpf caasapplicationprovisioner.CAASUnitProvisionerFacade, as caasapplicationprovisioner.ApplicationService, ss caasapplicationprovisioner.StatusService, l logger.Logger) error {
			settingsChan <- struct{}{}
			return nil
		}),

		// settingsChan fired
		ops.EXPECT().EnsureTrust(gomock.Any(), "test", app, applicationService, s.logger).Return(errors.NotFound),
		ops.EXPECT().EnsureTrust(gomock.Any(), "test", app, applicationService, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ caasapplicationprovisioner.ApplicationService, _ logger.Logger) error {
			appUnitsChan <- nil
			return nil
		}),

		// appUnitsChan fired
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", s.appID, app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.NotFound),
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", s.appID, app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.ConstError("try again")),
		ops.EXPECT().ReconcileDeadUnitScale(gomock.Any(), "test", s.appID, app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, cf caasapplicationprovisioner.CAASProvisionerFacade, as caasapplicationprovisioner.ApplicationService, ss caasapplicationprovisioner.StatusService, l logger.Logger) error {
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
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Alive, nil),
		ops.EXPECT().AppAlive(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s1 string, a caas.Application, s2 string, ac *caas.ApplicationConfig, cf caasapplicationprovisioner.CAASProvisionerFacade, ss caasapplicationprovisioner.StatusService, c clock.Clock, l logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Dying, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", s.appID, app, life.Dying, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, v life.Value, cf caasapplicationprovisioner.CAASProvisionerFacade, cpf caasapplicationprovisioner.CAASUnitProvisionerFacade, as caasapplicationprovisioner.ApplicationService, ss caasapplicationprovisioner.StatusService, l logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		applicationService.EXPECT().GetApplicationLife(gomock.Any(), s.appID).Return(life.Dead, nil),
		ops.EXPECT().AppDying(gomock.Any(), "test", s.appID, app, life.Dead, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		ops.EXPECT().AppDead(gomock.Any(), "test", app, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, c1 caasapplicationprovisioner.CAASBroker, _ caasapplicationprovisioner.CAASProvisionerFacade, _ caasapplicationprovisioner.CAASUnitProvisionerFacade, c2 clock.Clock, _ logger.Logger) error {
			close(done)
			return nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, unitFacade, ops, applicationService, statusService)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}
