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
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	. "github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
)

func TestApplicationWorkerSuite(t *testing.T) {
	tc.Run(t, &ApplicationWorkerSuite{})
}

type ApplicationWorkerSuite struct {
	coretesting.BaseSuite

	appID  application.ID
	logger logger.Logger
}

func (s *ApplicationWorkerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.appID, err = application.NewID()
	c.Assert(err, tc.ErrorIsNil)

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
	facade CAASProvisionerFacade,
	broker CAASBroker,
	ops ApplicationOps,
	applicationService ApplicationService,
	statusService StatusService,
	agentPasswordService AgentPasswordService,
	storageProvisioningService StorageProvisioningService,
	resourceOpenerGetter ResourceOpenerGetter,
) worker.Worker {
	config := AppWorkerConfig{
		AppID:                      s.appID,
		Facade:                     facade,
		Broker:                     broker,
		Clock:                      clk,
		Logger:                     s.logger,
		Ops:                        ops,
		ApplicationService:         applicationService,
		StatusService:              statusService,
		AgentPasswordService:       agentPasswordService,
		StorageProvisioningService: storageProvisioningService,
		ResourceOpenerGetter:       resourceOpenerGetter,
	}
	startFunc := NewAppWorker(config)
	c.Assert(startFunc, tc.NotNil)
	appWorker, err := startFunc(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appWorker, tc.NotNil)
	return appWorker
}

func (s *ApplicationWorkerSuite) TestLifeNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	x := gomock.Any()

	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	agentPasswordService := mocks.NewMockAgentPasswordService(ctrl)
	storageProvisioningService := mocks.NewMockStorageProvisioningService(ctrl)
	resourceOpenerGetter := mocks.NewMockResourceOpenerGetter(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(x, s.appID).DoAndReturn(func(ctx context.Context, appID application.ID) (string, error) {
			close(done)
			return "", applicationerrors.ApplicationNotFound
		}),
	)
	appWorker := s.startAppWorker(c, nil, facade, broker, ops, applicationService, statusService, agentPasswordService, storageProvisioningService, resourceOpenerGetter)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestLifeDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	x := gomock.Any()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	agentPasswordService := mocks.NewMockAgentPasswordService(ctrl)
	storageProvisioningService := mocks.NewMockStorageProvisioningService(ctrl)
	resourceOpenerGetter := mocks.NewMockResourceOpenerGetter(ctrl)
	clk := testclock.NewDilatedWallClock(time.Millisecond)

	done := make(chan struct{})

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(x, s.appID).Return("test", nil),
		applicationService.EXPECT().IsControllerApplication(x, s.appID).Return(false, nil),
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Dead, nil),
		ops.EXPECT().AppDying(x, "test", s.appID, app, life.Dead, x, x, x, x).Return(nil),
		ops.EXPECT().AppDead(x, "test", s.appID, app, broker, applicationService, statusService, clk, s.logger).DoAndReturn(func(context.Context, string, application.ID, caas.Application, CAASBroker, ApplicationService, StatusService, clock.Clock, logger.Logger) error {
			close(done)
			return nil
		}),
	)
	appWorker := s.startAppWorker(c, clk, facade, broker, ops, applicationService, statusService, agentPasswordService, storageProvisioningService, resourceOpenerGetter)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestWorker(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	x := gomock.Any()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	agentPasswordService := mocks.NewMockAgentPasswordService(ctrl)
	storageProvisioningService := mocks.NewMockStorageProvisioningService(ctrl)
	resourceOpenerGetter := mocks.NewMockResourceOpenerGetter(ctrl)
	done := make(chan struct{})

	clk := testclock.NewDilatedWallClock(time.Millisecond)

	scaleChan := make(chan struct{}, 1)
	settingsChan := make(chan struct{}, 1)
	provisioningInfoChan := make(chan struct{}, 1)
	appUnitsChan := make(chan []string, 1)
	appChan := make(chan struct{}, 1)
	appReplicasChan := make(chan struct{}, 1)

	ops.EXPECT().RefreshApplicationStatus(x, "test", s.appID, app, x, x, x, x).Return(nil).AnyTimes()

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(x, s.appID).Return("test", nil),
		applicationService.EXPECT().IsControllerApplication(x, s.appID).Return(false, nil),
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Alive, nil),

		agentPasswordService.EXPECT().SetApplicationPassword(x, s.appID, x).Return(nil),

		applicationService.EXPECT().WatchApplicationScale(x, "test").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		applicationService.EXPECT().WatchApplicationSettings(x, "test").Return(watchertest.NewMockNotifyWatcher(settingsChan), nil),
		applicationService.EXPECT().WatchApplicationUnitLife(x, "test").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Alive, nil),
		applicationService.EXPECT().GetApplicationScalingState(x, "test").Return(applicationservice.ScalingState{}, nil),
		facade.EXPECT().WatchProvisioningInfo(x, "test").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		ops.EXPECT().ProvisioningInfo(x, "test", s.appID, x, x, x, x, x, x).Return(&ProvisioningInfo{}, nil),
		ops.EXPECT().AppAlive(x, "test", app, x, x, x, x, x, x).Return(nil),
		app.EXPECT().Watch(x).Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			scaleChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// scaleChan fired
		ops.EXPECT().EnsureScale(x, "test", s.appID, app, life.Alive, x, x, x, x).Return(errors.NotFound),
		ops.EXPECT().EnsureScale(x, "test", s.appID, app, life.Alive, x, x, x, x).Return(errors.ConstError("try again")),
		ops.EXPECT().EnsureScale(x, "test", s.appID, app, life.Alive, x, x, x, x).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, v life.Value, cf CAASProvisionerFacade, as ApplicationService, ss StatusService, l logger.Logger) error {
			settingsChan <- struct{}{}
			return nil
		}),

		// trustChan fired
		ops.EXPECT().EnsureTrust(x, "test", app, x, x).Return(errors.NotFound),
		ops.EXPECT().EnsureTrust(x, "test", app, x, x).DoAndReturn(func(ctx context.Context, s string, a caas.Application, as ApplicationService, l logger.Logger) error {
			appUnitsChan <- nil
			return nil
		}),

		// appUnitsChan fired
		ops.EXPECT().ReconcileDeadUnitScale(x, "test", s.appID, app, x, x, x, x).Return(errors.NotFound),
		ops.EXPECT().ReconcileDeadUnitScale(x, "test", s.appID, app, x, x, x, x).Return(errors.ConstError("try again")),
		ops.EXPECT().ReconcileDeadUnitScale(x, "test", s.appID, app, x, x, x, x).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, cf CAASProvisionerFacade, as ApplicationService, ss StatusService, l logger.Logger) error {
			appChan <- struct{}{}
			return nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState(x, "test", s.appID, app, x, x, x, x, x, x).DoAndReturn(func(context.Context, string, application.ID, caas.Application, UpdateStatusState, CAASBroker, ApplicationService, StatusService, clock.Clock, logger.Logger) (UpdateStatusState, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState(x, "test", s.appID, app, x, x, x, x, x, x).DoAndReturn(func(context.Context, string, application.ID, caas.Application, UpdateStatusState, CAASBroker, ApplicationService, StatusService, clock.Clock, logger.Logger) (UpdateStatusState, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Alive, nil),
		ops.EXPECT().ProvisioningInfo(x, "test", s.appID, x, x, x, x, x, x).Return(&ProvisioningInfo{}, nil),
		ops.EXPECT().AppAlive(x, "test", app, x, x, x, x, x, x).DoAndReturn(func(ctx context.Context, s1 string, a caas.Application, s2 string, ac *caas.ApplicationConfig, pi *ProvisioningInfo, ss StatusService, c clock.Clock, l logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Dying, nil),
		ops.EXPECT().AppDying(x, "test", s.appID, app, life.Dying, x, x, x, x).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, v life.Value, cf CAASProvisionerFacade, as ApplicationService, ss StatusService, l logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Dead, nil),
		ops.EXPECT().AppDying(x, "test", s.appID, app, life.Dead, x, x, x, x).Return(nil),
		ops.EXPECT().AppDead(x, "test", s.appID, app, x, x, x, x, x).DoAndReturn(func(context.Context, string, application.ID, caas.Application, CAASBroker, ApplicationService, StatusService, clock.Clock, logger.Logger) error {
			close(done)
			return nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, ops, applicationService, statusService, agentPasswordService, storageProvisioningService, resourceOpenerGetter)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestWorkerStatusOnly(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	x := gomock.Any()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	agentPasswordService := mocks.NewMockAgentPasswordService(ctrl)
	storageProvisioningService := mocks.NewMockStorageProvisioningService(ctrl)
	resourceOpenerGetter := mocks.NewMockResourceOpenerGetter(ctrl)
	done := make(chan struct{})

	clk := testclock.NewDilatedWallClock(time.Millisecond)

	scaleChan := make(chan struct{}, 1)
	settingsChan := make(chan struct{}, 1)
	provisioningInfoChan := make(chan struct{}, 1)
	appUnitsChan := make(chan []string, 1)
	appChan := make(chan struct{}, 1)
	appReplicasChan := make(chan struct{}, 1)

	ops.EXPECT().RefreshApplicationStatus(x, "con-troll-er", s.appID, app, x, x, x, x).Return(nil).AnyTimes()

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(x, s.appID).Return("con-troll-er", nil),
		applicationService.EXPECT().IsControllerApplication(x, s.appID).Return(true, nil),
		broker.EXPECT().Application("con-troll-er", caas.DeploymentStateful).Return(app),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Alive, nil),

		applicationService.EXPECT().WatchApplicationScale(x, "con-troll-er").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		applicationService.EXPECT().WatchApplicationSettings(x, "con-troll-er").Return(watchertest.NewMockNotifyWatcher(settingsChan), nil),
		applicationService.EXPECT().WatchApplicationUnitLife(x, "con-troll-er").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Alive, nil),
		applicationService.EXPECT().GetApplicationScalingState(x, "con-troll-er").Return(applicationservice.ScalingState{Scaling: true, ScaleTarget: 1}, nil),
		applicationService.EXPECT().SetApplicationScalingState(x, "con-troll-er", 0, false).Return(nil),
		facade.EXPECT().WatchProvisioningInfo(x, "con-troll-er").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		app.EXPECT().Watch(x).Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			appChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState(x, "con-troll-er", s.appID, app, x, broker, applicationService, statusService, clk, s.logger).DoAndReturn(func(context.Context, string, application.ID, caas.Application, UpdateStatusState, CAASBroker, ApplicationService, StatusService, clock.Clock, logger.Logger) (UpdateStatusState, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState(x, "con-troll-er", s.appID, app, x, broker, applicationService, statusService, clk, s.logger).DoAndReturn(func(context.Context, string, application.ID, caas.Application, UpdateStatusState, CAASBroker, ApplicationService, StatusService, clock.Clock, logger.Logger) (UpdateStatusState, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		applicationService.EXPECT().GetApplicationLife(x, s.appID).DoAndReturn(func(ctx context.Context, i application.ID) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			return life.Alive, nil
		}),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).DoAndReturn(func(ctx context.Context, i application.ID) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			return life.Dying, nil
		}),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).DoAndReturn(func(ctx context.Context, i application.ID) (life.Value, error) {
			provisioningInfoChan <- struct{}{}
			close(done)
			return life.Dead, nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, ops, applicationService, statusService, agentPasswordService, storageProvisioningService, resourceOpenerGetter)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestNotProvisionedRetry(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	x := gomock.Any()

	broker := mocks.NewMockCAASBroker(ctrl)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	agentPasswordService := mocks.NewMockAgentPasswordService(ctrl)
	storageProvisioningService := mocks.NewMockStorageProvisioningService(ctrl)
	resourceOpenerGetter := mocks.NewMockResourceOpenerGetter(ctrl)
	done := make(chan struct{})

	clk := testclock.NewDilatedWallClock(time.Millisecond)

	scaleChan := make(chan struct{}, 1)
	settingsChan := make(chan struct{}, 1)
	provisioningInfoChan := make(chan struct{}, 1)
	appUnitsChan := make(chan []string, 1)
	appChan := make(chan struct{}, 1)
	appReplicasChan := make(chan struct{}, 1)

	ops.EXPECT().RefreshApplicationStatus(x, "test", s.appID, app, x, x, x, x).Return(nil).AnyTimes()

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationName(x, s.appID).Return("test", nil),
		applicationService.EXPECT().IsControllerApplication(x, s.appID).Return(false, nil),
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(app),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Alive, nil),

		agentPasswordService.EXPECT().SetApplicationPassword(x, s.appID, x).Return(nil),

		applicationService.EXPECT().WatchApplicationScale(x, "test").Return(watchertest.NewMockNotifyWatcher(scaleChan), nil),
		applicationService.EXPECT().WatchApplicationSettings(x, "test").Return(watchertest.NewMockNotifyWatcher(settingsChan), nil),
		applicationService.EXPECT().WatchApplicationUnitLife(x, "test").Return(watchertest.NewMockStringsWatcher(appUnitsChan), nil),

		// handleChange
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Alive, nil),
		applicationService.EXPECT().GetApplicationScalingState(x, "test").Return(applicationservice.ScalingState{}, nil),
		facade.EXPECT().WatchProvisioningInfo(x, "test").Return(watchertest.NewMockNotifyWatcher(provisioningInfoChan), nil),
		// error with not provisioned
		ops.EXPECT().ProvisioningInfo(x, "test", s.appID, x, x, x, x, x, x).Return(nil, errors.NotProvisioned),

		// retry handleChange
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Alive, nil),
		ops.EXPECT().ProvisioningInfo(x, "test", s.appID, x, x, x, x, x, x).Return(&ProvisioningInfo{}, nil),
		ops.EXPECT().AppAlive(x, "test", app, x, x, x, x, x, x).Return(nil),
		app.EXPECT().Watch(x).Return(watchertest.NewMockNotifyWatcher(appChan), nil),
		app.EXPECT().WatchReplicas().DoAndReturn(func() (watcher.NotifyWatcher, error) {
			scaleChan <- struct{}{}
			return watchertest.NewMockNotifyWatcher(appReplicasChan), nil
		}),

		// scaleChan fired
		ops.EXPECT().EnsureScale(x, "test", s.appID, app, life.Alive, x, x, x, x).Return(errors.NotFound),
		ops.EXPECT().EnsureScale(x, "test", s.appID, app, life.Alive, x, x, x, x).Return(errors.ConstError("try again")),
		ops.EXPECT().EnsureScale(x, "test", s.appID, app, life.Alive, x, x, x, x).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, v life.Value, cf CAASProvisionerFacade, as ApplicationService, ss StatusService, l logger.Logger) error {
			settingsChan <- struct{}{}
			return nil
		}),

		// settingsChan fired
		ops.EXPECT().EnsureTrust(x, "test", app, applicationService, s.logger).Return(errors.NotFound),
		ops.EXPECT().EnsureTrust(x, "test", app, applicationService, s.logger).DoAndReturn(func(_ context.Context, _ string, _ caas.Application, _ ApplicationService, _ logger.Logger) error {
			appUnitsChan <- nil
			return nil
		}),

		// appUnitsChan fired
		ops.EXPECT().ReconcileDeadUnitScale(x, "test", s.appID, app, x, x, x, x).Return(errors.NotFound),
		ops.EXPECT().ReconcileDeadUnitScale(x, "test", s.appID, app, x, x, x, x).Return(errors.ConstError("try again")),
		ops.EXPECT().ReconcileDeadUnitScale(x, "test", s.appID, app, x, x, x, x).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, cf CAASProvisionerFacade, as ApplicationService, ss StatusService, l logger.Logger) error {
			appChan <- struct{}{}
			return nil
		}),

		// appChan fired
		ops.EXPECT().UpdateState(x, "test", s.appID, app, x, x, x, x, x, x).DoAndReturn(func(context.Context, string, application.ID, caas.Application, UpdateStatusState, CAASBroker, ApplicationService, StatusService, clock.Clock, logger.Logger) (UpdateStatusState, error) {
			appReplicasChan <- struct{}{}
			return nil, nil
		}),
		// appReplicasChan fired
		ops.EXPECT().UpdateState(x, "test", s.appID, app, x, x, x, x, x, x).DoAndReturn(func(context.Context, string, application.ID, caas.Application, UpdateStatusState, CAASBroker, ApplicationService, StatusService, clock.Clock, logger.Logger) (UpdateStatusState, error) {
			provisioningInfoChan <- struct{}{}
			return nil, nil
		}),

		// provisioningInfoChan fired
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Alive, nil),
		ops.EXPECT().ProvisioningInfo(x, "test", s.appID, x, x, x, x, x, x).Return(&ProvisioningInfo{}, nil),
		ops.EXPECT().AppAlive(x, "test", app, x, x, x, x, x, x).DoAndReturn(func(ctx context.Context, s1 string, a caas.Application, s2 string, ac *caas.ApplicationConfig, pi *ProvisioningInfo, ss StatusService, c clock.Clock, l logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Dying, nil),
		ops.EXPECT().AppDying(x, "test", s.appID, app, life.Dying, x, x, x, x).DoAndReturn(func(ctx context.Context, s string, i application.ID, a caas.Application, v life.Value, cf CAASProvisionerFacade, as ApplicationService, ss StatusService, l logger.Logger) error {
			provisioningInfoChan <- struct{}{}
			return nil
		}),
		applicationService.EXPECT().GetApplicationLife(x, s.appID).Return(life.Dead, nil),
		ops.EXPECT().AppDying(x, "test", s.appID, app, life.Dead, x, x, x, x).Return(nil),
		ops.EXPECT().AppDead(x, "test", s.appID, app, x, x, x, x, x).DoAndReturn(func(context.Context, string, application.ID, caas.Application, CAASBroker, ApplicationService, StatusService, clock.Clock, logger.Logger) error {
			close(done)
			return nil
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, ops, applicationService, statusService, agentPasswordService, storageProvisioningService, resourceOpenerGetter)
	s.waitDone(c, done)
	workertest.CheckKill(c, appWorker)
}
