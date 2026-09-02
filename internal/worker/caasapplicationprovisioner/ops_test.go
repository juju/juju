// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/application"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/deployment/charm"
	charmresource "github.com/juju/juju/domain/deployment/charm/resource"
	"github.com/juju/juju/domain/storageprovisioning"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
	provisionertypes "github.com/juju/juju/internal/worker/caasapplicationprovisioner/types"
)

func TestOpsSuite(t *testing.T) {
	tc.Run(t, &OpsSuite{})
}

type OpsSuite struct {
	coretesting.BaseSuite

	modelTag names.ModelTag
	logger   logger.Logger
}

func (s *OpsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggertesting.WrapCheckLog(c)
}

func (s *OpsSuite) TestEnsureTrust(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	applicationService := mocks.NewMockApplicationService(ctrl)
	app := caasmocks.NewMockApplication(ctrl)

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationTrustSetting(gomock.Any(), "test").Return(true, nil),
		app.EXPECT().Trust(true).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureTrust(c.Context(), "test", app, applicationService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestUpdateState(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	broker := mocks.NewMockCAASBroker(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	now := time.Now()
	clk := testclock.NewClock(now)

	service := &caas.Service{
		Id: "provider-id",
		Status: status.StatusInfo{
			Status:  status.Active,
			Message: "nice message",
			Data: map[string]any{
				"nice": "data",
			},
		},
		Addresses: network.ProviderAddresses{{
			MachineAddress: network.NewMachineAddress("1.2.3.4"),
			SpaceName:      "space-name",
		}},
	}
	units := []caas.Unit{{
		Id:       "a",
		Address:  "1.2.3.5",
		Ports:    []string{"80", "443"},
		Stateful: true,
		Status: status.StatusInfo{
			Status:  status.Running,
			Message: "different",
		},
		FilesystemInfo: []caas.FilesystemInfo{{
			StorageName:               "s",
			PersistentVolumeClaimName: "fsid",
			Volume: caas.VolumeInfo{
				PersistentVolumeName: "vid",
			},
		}},
	}, {
		Id:       "b",
		Address:  "1.2.3.6",
		Ports:    []string{"80", "443"},
		Stateful: true,
		Status: status.StatusInfo{
			Status:  status.Allocating,
			Message: "same",
		},
	}}
	appStatus := status.StatusInfo{
		Status:  status.Active,
		Message: "nice message",
		Data: map[string]any{
			"nice": "data",
		},
		Since: &now,
	}
	k8sPodIDs := map[unit.Name]string{
		"test/0": "a",
		"test/1": "b",
	}

	unit0Update := applicationservice.UpdateCAASUnitParams{
		ProviderID: new("a"),
		Address:    new("1.2.3.5"),
		Ports:      new([]string{"80", "443"}),
		AgentStatus: &status.StatusInfo{
			Status: status.Idle,
			Since:  &now,
		},
		K8sPodStatus: &status.StatusInfo{
			Status:  status.Running,
			Message: "different",
			Since:   &now,
		},
	}

	gomock.InOrder(
		app.EXPECT().Service().Return(service, nil),
		applicationService.EXPECT().UpdateK8sService(gomock.Any(), "test", "provider-id", network.ProviderAddresses{{
			MachineAddress: network.NewMachineAddress("1.2.3.4"),
			SpaceName:      "space-name",
		}}).Return(nil),
		statusService.EXPECT().SetOperatorStatus(gomock.Any(), "test", appStatus).Return(nil),
		applicationService.EXPECT().GetAllUnitK8sPodIDsForApplication(gomock.Any(), appId).Return(k8sPodIDs, nil),
		app.EXPECT().Units().Return(units, nil),
		applicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unit.Name("test/0"), gomock.Any()).DoAndReturn(func(_ context.Context, _ unit.Name, args applicationservice.UpdateCAASUnitParams) error {
			c.Check(args.ProviderID, tc.DeepEquals, unit0Update.ProviderID)
			c.Check(args.Address, tc.DeepEquals, unit0Update.Address)
			c.Check(args.Ports, tc.DeepEquals, unit0Update.Ports)
			c.Assert(args.AgentStatus, tc.NotNil, tc.Commentf("AgentStatus should not be nil"))
			c.Assert(args.AgentStatus.Since, tc.NotNil, tc.Commentf("AgentStatus.Since should not be nil"))
			c.Check(*args.AgentStatus.Since, tc.Equals, now, tc.Commentf("AgentStatus.Since should be set to current time"))
			c.Assert(args.K8sPodStatus, tc.NotNil, tc.Commentf("K8sPodStatus should not be nil"))
			c.Assert(args.K8sPodStatus.Since, tc.NotNil, tc.Commentf("K8sPodStatus.Since should not be nil"))
			c.Check(*args.K8sPodStatus.Since, tc.Equals, now, tc.Commentf("K8sPodStatus.Since should be set to current time"))
			return nil
		}),
		broker.EXPECT().AnnotateUnit(gomock.Any(), "test", "a", names.NewUnitTag("test/0")).Return(nil),
	)

	lastReportedStatus := caasapplicationprovisioner.UpdateStatusState{
		"test/1": {
			ProviderID: new("b"),
			Address:    new("1.2.3.6"),
			Ports:      new([]string{"80", "443"}),
			AgentStatus: &status.StatusInfo{
				Status:  status.Allocating,
				Message: "same",
				Since:   &now,
			},
			K8sPodStatus: &status.StatusInfo{
				Status:  status.Waiting,
				Message: "same",
				Since:   &now,
			},
		},
	}
	currentReportedStatus, err := caasapplicationprovisioner.AppOps.UpdateState(c.Context(), "test", appId, app, lastReportedStatus, broker, applicationService, statusService, clk, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(currentReportedStatus, tc.DeepEquals, caasapplicationprovisioner.UpdateStatusState{
		"test/0": {
			ProviderID: new("a"),
			Address:    new("1.2.3.5"),
			Ports:      new([]string{"80", "443"}),
			AgentStatus: &status.StatusInfo{
				Status: status.Idle,
				Since:  &now,
			},
			K8sPodStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "different",
				Since:   &now,
			},
		},
		"test/1": {
			ProviderID: new("b"),
			Address:    new("1.2.3.6"),
			Ports:      new([]string{"80", "443"}),
			AgentStatus: &status.StatusInfo{
				Status:  status.Allocating,
				Message: "same",
				Since:   &now,
			},
			K8sPodStatus: &status.StatusInfo{
				Status:  status.Waiting,
				Message: "same",
				Since:   &now,
			},
		},
	})
}

func (s *OpsSuite) TestRefreshOperatorStatusChurningAllocating(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appLife := life.Alive
	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	appState := caas.ApplicationState{
		DesiredReplicas: 2,
	}
	units := map[unit.Name]status.StatusInfo{
		"test/0": {
			Status: status.Active,
		},
		"test/1": {
			Status: status.Allocating,
		},
	}
	gomock.InOrder(
		app.EXPECT().State().Return(appState, nil),
		statusService.EXPECT().GetUnitAgentStatusesForApplication(gomock.Any(), appId).Return(units, nil),
		statusService.EXPECT().SetOperatorStatus(gomock.Any(), "test", gomock.Any()).DoAndReturn(func(ctx context.Context, name string, si status.StatusInfo) error {
			mc := tc.NewMultiChecker()
			mc.AddExpr("_.Since", tc.NotNil)
			c.Check(si, mc, status.StatusInfo{
				Status:  status.Waiting,
				Message: "waiting for units to settle down",
			})
			return nil
		}),
	)

	err := caasapplicationprovisioner.AppOps.RefreshOperatorStatus(c.Context(), "test", appId, app, appLife, statusService, clk, s.logger)
	c.Assert(errors.Is(err, errors.ConstError("units churning")), tc.IsTrue)
}

func (s *OpsSuite) TestRefreshApplicationStatusSettled(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appLife := life.Alive
	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	appState := caas.ApplicationState{
		DesiredReplicas: 2,
	}
	units := map[unit.Name]status.StatusInfo{
		"test/0": {
			Status: status.Active,
		},
		"test/1": {
			Status: status.Executing,
		},
		"test/2": {
			Status: status.Waiting,
		},
	}
	gomock.InOrder(
		app.EXPECT().State().Return(appState, nil),
		statusService.EXPECT().GetUnitAgentStatusesForApplication(gomock.Any(), appId).Return(units, nil),
		statusService.EXPECT().SetOperatorStatus(gomock.Any(), "test", gomock.Any()).DoAndReturn(func(ctx context.Context, name string, si status.StatusInfo) error {
			mc := tc.NewMultiChecker()
			mc.AddExpr("_.Since", tc.NotNil)
			c.Check(si, mc, status.StatusInfo{
				Status: status.Active,
			})
			return nil
		}),
	)

	err := caasapplicationprovisioner.AppOps.RefreshOperatorStatus(c.Context(), "test", appId, app, appLife, statusService, clk, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestRefreshOperatorStatusChurningWaitingInitialising(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appLife := life.Alive
	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	appState := caas.ApplicationState{
		DesiredReplicas: 2,
	}
	units := map[unit.Name]status.StatusInfo{
		"test/0": {
			Status: status.Active,
		},
		"test/1": {
			Status: status.Waiting, Message: "agent initialising",
		},
	}
	gomock.InOrder(
		app.EXPECT().State().Return(appState, nil),
		statusService.EXPECT().GetUnitAgentStatusesForApplication(gomock.Any(), appId).Return(units, nil),
		statusService.EXPECT().SetOperatorStatus(gomock.Any(), "test", gomock.Any()).DoAndReturn(func(ctx context.Context, name string, si status.StatusInfo) error {
			mc := tc.NewMultiChecker()
			mc.AddExpr("_.Since", tc.NotNil)
			c.Check(si, mc, status.StatusInfo{
				Status:  status.Waiting,
				Message: "waiting for units to settle down",
			})
			return nil
		}),
	)

	err := caasapplicationprovisioner.AppOps.RefreshOperatorStatus(c.Context(), "test", appId, app, appLife, statusService, clk, s.logger)
	c.Assert(errors.Is(err, errors.ConstError("units churning")), tc.IsTrue)
}

func (s *OpsSuite) TestWaitForTerminated(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	gomock.InOrder(
		app.EXPECT().Exists().Return(caas.DeploymentState{
			Exists: true,
		}, nil),
	)
	err := caasapplicationprovisioner.AppOps.WaitForTerminated("test", app, clk)
	c.Assert(err, tc.ErrorMatches, `application "test" should be terminating but is now running`)

	gomock.InOrder(
		app.EXPECT().Exists().Return(caas.DeploymentState{
			Exists:      true,
			Terminating: true,
		}, nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
	)
	err = caasapplicationprovisioner.AppOps.WaitForTerminated("test", app, clk)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestReconcileDeadUnitScale(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	units := map[unit.Name]life.Value{
		"test/0": life.Dead,
		"test/1": life.Alive,
	}
	ps := applicationservice.ScalingState{
		StartOrdinal: 2,
		Scaling:      true,
		ScaleTarget:  1,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
		facade.EXPECT().RemoveUnit(gomock.Any(), "test/0").Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test",
		appUUID, app, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorMatches, `try again`)
}

func (s *OpsSuite) TestReconcileDeadUnitScaleScalesAfterUnitRemoval(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	storageUniqueID := appUUID.String()[:6]
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	units := map[unit.Name]life.Value{
		"test/1": life.Alive,
	}
	ps := applicationservice.ScalingState{
		StartOrdinal: 1,
		Scaling:      true,
		ScaleTarget:  1,
	}

	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
		facade.EXPECT().FilesystemProvisioningInfo(gomock.Any(), "test").Return(provisionertypes.FilesystemProvisioningInfo{}, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any(), storageUniqueID),
		app.EXPECT().ScaleRange(gomock.Any(), 1, 1).Return(nil),
		app.EXPECT().State().Return(caas.ApplicationState{Replicas: []string{"test/1"}}, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 0, 1, false).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test",
		appUUID, app, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestReconcileDeadUnitScaleMultipleLowestDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale down from 4 to 2: test/0 and test/1 are dead and are the
	// lowest ordinals, so they should be removed.
	units := map[unit.Name]life.Value{
		"test/0": life.Dead,
		"test/1": life.Dead,
		"test/2": life.Alive,
		"test/3": life.Alive,
	}
	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 2,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
		facade.EXPECT().RemoveUnit(gomock.Any(), "test/0").Return(nil),
		facade.EXPECT().RemoveUnit(gomock.Any(), "test/1").Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test",
		appUUID, app, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorMatches, `try again`)
}

func (s *OpsSuite) TestReconcileDeadUnitScaleLowestNotDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale down from 3 to 1: test/0 is alive so it can't be removed
	// yet. Should be a no-op.
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Dead,
		"test/2": life.Alive,
	}
	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 1,
	}

	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test",
		appId, app, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestReconcileDeadUnitScaleAllLowestDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale down from 3 to 1: test/0 and test/1 are dead, test/2 is kept.
	units := map[unit.Name]life.Value{
		"test/0": life.Dead,
		"test/1": life.Dead,
		"test/2": life.Alive,
	}
	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 1,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
		facade.EXPECT().RemoveUnit(gomock.Any(), "test/0").Return(nil),
		facade.EXPECT().RemoveUnit(gomock.Any(), "test/1").Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test",
		appUUID, app, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorMatches, `try again`)
}

// TestReconcileDeadUnitScaleScaleUp tests scale up scenario - app.Scale should NOT be called
func (s *OpsSuite) TestReconcileDeadUnitScaleScaleUp(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale DOWN: 4 current units -> 2 target units, all excess units are dead
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Dead,
	}
	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 5, // Scale up to 5 units
	}

	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
	)
	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test",
		appId, app, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

// TestReconcileDeadUnitScaleScaleDownNotAllDead tests scale down when not all excess units are dead - app.Scale should NOT be called
func (s *OpsSuite) TestReconcileDeadUnitScaleScaleDownNotAllDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale DOWN: 4 current units -> 2 target units, all excess units are dead
	units := map[unit.Name]life.Value{
		"test/0": life.Alive, // < keep threshold
		"test/1": life.Dead,  // < keep threshold
		"test/2": life.Dead,
		"test/3": life.Alive,
	}
	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 2, // Scale down to 2 units
	}

	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test",
		appId, app, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestReconcileDeadUnitScaleSparseOrdinals(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Ordinals are sparse: {0, 2, 4} (missing 1, 3). Scale down to 2.
	// Keep the highest 2 ordinals {2, 4}, remove {0}. test/0 is dead.
	units := map[unit.Name]life.Value{
		"test/0": life.Dead,
		"test/2": life.Alive,
		"test/4": life.Alive,
	}
	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 2,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
		facade.EXPECT().RemoveUnit(gomock.Any(), "test/0").Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test",
		appUUID, app, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorMatches, `try again`)
}

func (s *OpsSuite) TestReconcileDeadUnitScaleSparseLowestNotDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Ordinals are sparse: {0, 2, 4}. Scale down to 1.
	// Keep {4}, remove {0, 2}. test/0 is alive → no-op.
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/2": life.Dead,
		"test/4": life.Dead,
	}
	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 1,
	}

	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test",
		appId, app, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAlive(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
		"test/2": life.Dying,
		"test/3": life.Dead,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(1, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 1, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 1, 3, true).Return(nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), gomock.InAnyOrder([]string{"test/0", "test/1"})).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveRetry(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 1,
	}
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
		"test/2": life.Dying,
		"test/3": life.Dead,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(10, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), gomock.InAnyOrder([]string{"test/0", "test/1"})).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorMatches, `try again`)
}

func (s *OpsSuite) TestEnsureScaleControllerReusesPersistedNonce(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	storageUniqueID := appID.String()[:6]
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	agentPasswordService := mocks.NewMockAgentPasswordService(ctrl)

	units := map[unit.Name]life.Value{
		"controller/0": life.Alive,
	}

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "controller").Return(2, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "controller").Return(applicationservice.ScalingState{
			Scaling:     true,
			ScaleTarget: 2,
		}, nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).Return(units, nil),
		applicationService.EXPECT().IsControllerApplication(gomock.Any(), appID).Return(true, nil),
		agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "0", gomock.Any()).Return("controller-0-nonce", nil),
		app.EXPECT().EnsureControllerNonce(gomock.Any(), 0, "controller-0-nonce").Return(nil),
		agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "1", gomock.Any()).Return("persisted-nonce", nil),
		app.EXPECT().EnsureControllerNonce(gomock.Any(), 1, "persisted-nonce").Return(nil),
		facade.EXPECT().FilesystemProvisioningInfo(gomock.Any(), "controller").Return(provisionertypes.FilesystemProvisioningInfo{}, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any(), storageUniqueID).Return(nil),
		app.EXPECT().Scale(gomock.Any(), 2).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "controller", appID, app,
		life.Alive, facade, applicationService, agentPasswordService, s.logger)
	c.Assert(err, tc.ErrorMatches, `try again`)
}

func (s *OpsSuite) TestEnsureScaleControllerReconcilesSparseOrdinals(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	storageUniqueID := appID.String()[:6]
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	agentPasswordService := mocks.NewMockAgentPasswordService(ctrl)

	units := map[unit.Name]life.Value{
		"controller/0": life.Alive,
		"controller/2": life.Alive,
	}

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "controller").Return(3, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "controller").Return(applicationservice.ScalingState{
			Scaling:     true,
			ScaleTarget: 3,
		}, nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).Return(units, nil),
		applicationService.EXPECT().IsControllerApplication(gomock.Any(), appID).Return(true, nil),
		agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "0", gomock.Any()).Return("controller-0-nonce", nil),
		app.EXPECT().EnsureControllerNonce(gomock.Any(), 0, "controller-0-nonce").Return(nil),
		agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "1", gomock.Any()).Return("controller-1-nonce", nil),
		app.EXPECT().EnsureControllerNonce(gomock.Any(), 1, "controller-1-nonce").Return(nil),
		agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "2", gomock.Any()).Return("controller-2-nonce", nil),
		app.EXPECT().EnsureControllerNonce(gomock.Any(), 2, "controller-2-nonce").Return(nil),
		facade.EXPECT().FilesystemProvisioningInfo(gomock.Any(), "controller").Return(provisionertypes.FilesystemProvisioningInfo{}, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any(), storageUniqueID).Return(nil),
		app.EXPECT().Scale(gomock.Any(), 3).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "controller", appID, app,
		life.Alive, facade, applicationService, agentPasswordService, s.logger)
	c.Assert(err, tc.ErrorMatches, `try again`)
}

func (s *OpsSuite) TestEnsureScaleControllerReconcilesNonceAfterUnitIntroduction(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	storageUniqueID := appID.String()[:6]
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	agentPasswordService := mocks.NewMockAgentPasswordService(ctrl)

	units := map[unit.Name]life.Value{
		"controller/0": life.Alive,
		"controller/1": life.Alive,
	}

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "controller").Return(2, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "controller").Return(applicationservice.ScalingState{
			Scaling:     true,
			ScaleTarget: 2,
		}, nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).Return(units, nil),
		applicationService.EXPECT().IsControllerApplication(gomock.Any(), appID).Return(true, nil),
		agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "0", gomock.Any()).Return("controller-0-nonce", nil),
		app.EXPECT().EnsureControllerNonce(gomock.Any(), 0, "controller-0-nonce").Return(nil),
		agentPasswordService.EXPECT().EnsureControllerNodeNonce(gomock.Any(), "1", gomock.Any()).Return("controller-1-nonce", nil),
		app.EXPECT().EnsureControllerNonce(gomock.Any(), 1, "controller-1-nonce").Return(nil),
		facade.EXPECT().FilesystemProvisioningInfo(gomock.Any(), "controller").Return(provisionertypes.FilesystemProvisioningInfo{}, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any(), storageUniqueID).Return(nil),
		app.EXPECT().Scale(gomock.Any(), 2).Return(nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "controller", 0, false).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "controller", appID, app,
		life.Alive, facade, applicationService, agentPasswordService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveScaleDown5To3(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale down from 5 to 3: destroy the lowest 2 ordinals (test/0, test/1).
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
		"test/2": life.Alive,
		"test/3": life.Alive,
		"test/4": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(3, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 3, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 3, 2, true).Return(nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), gomock.InAnyOrder([]string{"test/0", "test/1"})).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleResumesScaleDownWithMissingStartOrdinal(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
		"test/2": life.Alive,
	}

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(2, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{
			Scaling:     true,
			ScaleTarget: 2,
		}, nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).Return(units, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 2, 1, true).Return(nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), []string{"test/0"}).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appID, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveScaleDown5To2(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale down from 5 to 2: destroy the lowest 3 ordinals.
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
		"test/2": life.Alive,
		"test/3": life.Alive,
		"test/4": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(2, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 2, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 2, 3, true).Return(nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), gomock.InAnyOrder([]string{"test/0", "test/1", "test/2"})).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveScaleDown3To1(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale down from 3 to 1: destroy test/0, test/1.
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
		"test/2": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(1, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 1, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 1, 2, true).Return(nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), gomock.InAnyOrder([]string{"test/0", "test/1"})).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveScaleDownMixedLives(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale down from 5 to 2: some units are dying/dead, but only Alive
	// units in the lowest ordinal range should be destroyed.
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Dying,
		"test/2": life.Alive,
		"test/3": life.Dead,
		"test/4": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(2, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 2, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 2, 3, true).Return(nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), gomock.InAnyOrder([]string{"test/0", "test/2"})).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveScaleDownNothingToDestroy(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Scale down from 3 to 1, but the lowest ordinals are already
	// dying/dead, so there are no Alive units to destroy.
	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 1,
	}
	units := map[unit.Name]life.Value{
		"test/0": life.Dying,
		"test/1": life.Dead,
		"test/2": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(1, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 1, 2, true).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveScaleDownSparseOrdinals(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Ordinals are sparse: {0, 2, 4} (missing 1, 3). Scale down to 2.
	// Keep the highest 2 ordinals {2, 4}, destroy {0}.
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/2": life.Alive,
		"test/4": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(2, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 2, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 2, 2, true).Return(nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), gomock.InAnyOrder([]string{"test/0"})).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveScaleDownSparseOrdinals2(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Ordinals are sparse: {0, 1, 2, 4} (missing 3). Scale down to 2.
	// Keep the highest 2 ordinals {2, 4}, destroy {0, 1}.
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
		"test/2": life.Alive,
		"test/4": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(2, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 2, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().SetApplicationScalingStateWithStart(gomock.Any(), "test", 2, 2, true).Return(nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), gomock.InAnyOrder([]string{"test/0", "test/1"})).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleDyingDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	units := map[unit.Name]life.Value{
		"test/0": life.Dying,
		"test/1": life.Dead,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 0, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app,
		life.Dead, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleWithAttachStorage(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	storageUniqueID := appUUID.String()[:6]
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Current units (less than scale target)
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
	}

	// FilesystemProvisioningInfo with filesystem attachments
	provisioningInfo := provisionertypes.FilesystemProvisioningInfo{
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
			Provider:    storage.ProviderType("kubernetes"),
		}},
	}

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(2, nil),
		// Test scenario where we need to scale up and have attached storage
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{
			Scaling:     true,
			ScaleTarget: 2,
		}, nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(units, nil),
		applicationService.EXPECT().IsControllerApplication(gomock.Any(), appUUID).Return(false, nil),
		facade.EXPECT().FilesystemProvisioningInfo(gomock.Any(), "test").Return(provisioningInfo, nil),
		app.EXPECT().EnsurePVCs([]storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
			Provider:    "kubernetes",
		}}, gomock.Any(), storageUniqueID).Return(nil),
		app.EXPECT().Scale(gomock.Any(), 2).Return(nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 0, false).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appUUID, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleWithAttachStorageEnsurePVCsFails(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	storageUniqueID := appUUID.String()[:6]
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	// Current units (less than scale target)
	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
	}

	// FilesystemProvisioningInfo with filesystem attachments
	provisioningInfo := provisionertypes.FilesystemProvisioningInfo{
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
			Provider:    storage.ProviderType("kubernetes"),
		}},
	}

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(2, nil),
		// Test scenario where we need to scale up and have attached storage
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{
			Scaling:     true,
			ScaleTarget: 2,
		}, nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(units, nil),
		applicationService.EXPECT().IsControllerApplication(gomock.Any(), appUUID).Return(false, nil),
		facade.EXPECT().FilesystemProvisioningInfo(gomock.Any(), "test").Return(provisioningInfo, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any(), storageUniqueID).
			Return(errors.New("PVC creation failed")),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appUUID, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorMatches, "PVC creation failed")
}

func (s *OpsSuite) TestAppAlive(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)
	password := "123456789"
	lastApplied := caas.ApplicationConfig{}
	appUUID := tc.Must(c, application.NewUUID)
	storageUniqueID := appUUID.String()[:6]

	pi := caasapplicationprovisioner.ProvisioningInfo{
		ImageDetails: coreresource.DockerImageDetails{
			RegistryPath: "test-repo/jujud-operator:2.9.99",
			ImageRepoDetails: coreresource.ImageRepoDetails{
				Repository:    "test-repo",
				ServerAddress: "registry.com",
			},
		},
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "22.04",
				Risk:  corebase.Stable,
			},
		},
		Version:              semversion.MustParse("2.9.99"),
		CharmModifiedVersion: 123,
		APIAddresses:         []string{"1.2.3.1", "1.2.3.2", "1.2.3.3"},
		CACert:               "CACERT",
		Tags: map[string]string{
			"tag": "tag-value",
		},
		Trust:       true,
		Scale:       10,
		Constraints: constraints.MustParse("mem=1G"),
		Devices:     []devices.KubernetesDeviceParams{},
		CharmMeta: &charm.Meta{
			Containers: map[string]charm.Container{
				"mysql": {
					Resource: "mysql-image",
					Mounts: []charm.Mount{{
						Storage:  "data",
						Location: "/container-defined-location",
					}},
				},
				"rootless": {
					Resource: "rootless-image",
					Uid:      new(5000),
					Gid:      new(5001),
				},
			},
		},
		Images: map[string]coreresource.DockerImageDetails{
			"mysql-image": {
				RegistryPath: "mysql/ubuntu:latest-22.04",
			},
			"rootless-image": {
				RegistryPath: "rootless:foo-bar",
			},
		},
		FilesystemTemplates: []storageprovisioning.FilesystemTemplate{{
			Attachments: []storageprovisioning.FilesystemAttachmentTemplateWithProvisioned{
				{
					FilesystemAttachmentTemplate: storageprovisioning.FilesystemAttachmentTemplate{
						MountPoint: "/charm-defined-location/data/0",
						ReadOnly:   false,
					},
				},
			},
			StorageName:  "data",
			Count:        1,
			SizeMiB:      100,
			ProviderType: "kubernetes",
			Attributes: map[string]string{
				"attr-foo": "attr-bar",
			},
		}},
		StorageResourceTags: map[string]string{
			"rsc-foo": "rsc-bar",
		},
	}
	ds := caas.DeploymentState{
		Exists:      true,
		Terminating: true,
	}

	ensureParams := caas.ApplicationConfig{
		AgentVersion:         semversion.Number{Major: 2, Minor: 9, Patch: 99},
		AgentImagePath:       "test-repo/jujud-operator:2.9.99",
		CharmBaseImagePath:   "test-repo/charm-base:ubuntu-22.04",
		CharmModifiedVersion: 123,
		Containers: map[string]caas.ContainerConfig{
			"mysql": {
				Name: "mysql",
				Image: coreresource.DockerImageDetails{
					RegistryPath: "mysql/ubuntu:latest-22.04",
				},
				Mounts: []caas.MountConfig{{
					StorageName: "data",
					Path:        "/container-defined-location",
				}},
			},
			"rootless": {
				Name: "rootless",
				Image: coreresource.DockerImageDetails{
					RegistryPath: "rootless:foo-bar",
				},
				Uid: new(5000),
				Gid: new(5001),
			},
		},
		IntroductionSecret:   "123456789",
		ControllerAddresses:  "1.2.3.1,1.2.3.2,1.2.3.3",
		ControllerCertBundle: "CACERT",
		ResourceTags: map[string]string{
			"tag": "tag-value",
		},
		Constraints: constraints.MustParse("mem=1G"),
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
			Provider:    "kubernetes",
			Attributes: map[string]any{
				"attr-foo": "attr-bar",
			},
			ResourceTags: map[string]string{
				"rsc-foo": "rsc-bar",
			},
			Attachments: []storage.KubernetesFilesystemAttachmentParams{
				{
					ReadOnly: false,
					Path:     "/charm-defined-location/data/0",
				},
			},
		}},
		Devices:         []devices.KubernetesDeviceParams{},
		Trust:           true,
		InitialScale:    0,
		CharmUser:       caas.RunAsDefault,
		StorageUniqueID: storageUniqueID,
	}
	gomock.InOrder(
		app.EXPECT().Exists().Return(ds, nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		app.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
			c.Check(config, tc.DeepEquals, ensureParams)
			return nil
		}),
	)

	err := caasapplicationprovisioner.AppOps.AppAlive(c.Context(), "test",
		appUUID, app, password, &lastApplied, &pi, statusService, clk, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestAppAliveController(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)
	appUUID := tc.Must(c, application.NewUUID)
	lastApplied := caas.ApplicationConfig{}
	pi := caasapplicationprovisioner.ProvisioningInfo{
		ImageDetails: coreresource.DockerImageDetails{
			RegistryPath: "test-repo/jujud-operator:2.9.99",
			ImageRepoDetails: coreresource.ImageRepoDetails{
				Repository: "test-repo",
			},
		},
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "22.04",
				Risk:  corebase.Stable,
			},
		},
		Version: semversion.MustParse("2.9.99"),
		CharmMeta: &charm.Meta{
			CharmUser: charm.RunAsRoot,
		},
	}

	gomock.InOrder(
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		app.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
			c.Check(config.Controller, tc.IsTrue)
			c.Check(config.CharmUser, tc.Equals, caas.RunAsNonRoot)
			return nil
		}),
	)

	err := caasapplicationprovisioner.AppOps.AppAlive(c.Context(), application.ControllerApplicationName,
		appUUID, app, "password", &lastApplied, &pi, statusService,
		testclock.NewDilatedWallClock(coretesting.ShortWait), s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestAppDying(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	storageUniqueID := appUUID.String()[:6]
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 0, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(nil, nil),
		facade.EXPECT().FilesystemProvisioningInfo(gomock.Any(), "test").Return(provisionertypes.FilesystemProvisioningInfo{}, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any(), storageUniqueID).Return(nil),
		app.EXPECT().Scale(gomock.Any(), 0).Return(nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 0, false).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(nil, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
	)

	err := caasapplicationprovisioner.AppOps.AppDying(c.Context(), "test", appUUID, app,
		life.Dying, facade, applicationService, statusService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestAppDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	appUUID := tc.Must(c, application.NewUUID)

	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	gomock.InOrder(
		app.EXPECT().Delete().Return(nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		applicationService.EXPECT().ClearApplicationHasK8sResources(gomock.Any(), appUUID).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.AppDead(c.Context(), "test", appUUID, app, applicationService, clk, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestProvisioningInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewUUID()
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	storageProvisioningService := mocks.NewMockStorageProvisioningService(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	resourceOpenerGetter := mocks.NewMockResourceOpenerGetter(ctrl)
	ro := mocks.NewMockOpener(ctrl)
	resourceOpenerGetter.EXPECT().ResourceOpenerForApplication(gomock.Any(), appId, "test").Return(ro, nil)

	facadePi := provisionertypes.ProvisioningInfo{
		ImageDetails: coreresource.DockerImageDetails{
			RegistryPath: "test-repo/jujud-operator:2.9.99",
			ImageRepoDetails: coreresource.ImageRepoDetails{
				Repository:    "test-repo",
				ServerAddress: "registry.com",
			},
		},
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "22.04",
				Risk:  corebase.Stable,
			},
		},
		Version:              semversion.MustParse("2.9.99"),
		CharmModifiedVersion: 123,
		APIAddresses:         []string{"1.2.3.1", "1.2.3.2", "1.2.3.3"},
		CACert:               "CACERT",
		Tags: map[string]string{
			"tag": "tag-value",
		},
		Trust:       true,
		Scale:       10,
		Constraints: constraints.MustParse("mem=1G"),
		Devices:     []devices.KubernetesDeviceParams{},
	}
	facade.EXPECT().ProvisioningInfo(gomock.Any(), "test").Return(facadePi, nil)

	fsTemplates := []storageprovisioning.FilesystemTemplate{{
		StorageName:  "data",
		Count:        1,
		SizeMiB:      100,
		ProviderType: "kubernetes",
		Attributes: map[string]string{
			"attr-foo": "attr-bar",
		},
	}}
	storageProvisioningService.EXPECT().GetFilesystemTemplatesForApplication(gomock.Any(), appId).Return(fsTemplates, nil)

	storageResourceTags := map[string]string{
		"rsc-foo": "rsc-bar",
	}
	storageProvisioningService.EXPECT().GetStorageResourceTagsForApplication(gomock.Any(), appId).Return(storageResourceTags, nil)

	chMeta := &charm.Meta{
		Containers: map[string]charm.Container{
			"mysql": {
				Resource: "mysql-image",
				Mounts: []charm.Mount{{
					Storage:  "data",
					Location: "/container-defined-location",
				}},
			},
			"rootless": {
				Resource: "rootless-image",
				Uid:      new(5000),
				Gid:      new(5001),
			},
		},
		Resources: map[string]charmresource.Meta{
			"mysql-image": {
				Name: "mysql-image",
				Type: charmresource.TypeContainerImage,
			},
			"rootless-image": {
				Name: "rootless-image",
				Type: charmresource.TypeContainerImage,
			},
		},
	}
	ch := charm.NewCharmBase(chMeta, nil, nil, nil)
	applicationService.EXPECT().GetCharmByApplicationUUID(gomock.Any(), appId).Return(ch, applicationcharm.CharmLocator{}, nil)

	mysqlImageResource := coreresource.Opened{
		ReadCloser: io.NopCloser(bytes.NewBufferString("registrypath: mysql/ubuntu:latest-22.04")),
	}
	ro.EXPECT().OpenResource(gomock.Any(), "mysql-image").Return(mysqlImageResource, nil)
	ro.EXPECT().SetResourceUsed(gomock.Any(), gomock.Any()).Return(nil)
	rootlessImageResource := coreresource.Opened{
		ReadCloser: io.NopCloser(bytes.NewBufferString("registrypath: rootless:foo-bar")),
	}
	ro.EXPECT().OpenResource(gomock.Any(), "rootless-image").Return(rootlessImageResource, nil)
	ro.EXPECT().SetResourceUsed(gomock.Any(), gomock.Any()).Return(nil)

	pi, err := caasapplicationprovisioner.AppOps.ProvisioningInfo(c.Context(), "test", appId,
		facade, applicationService, storageProvisioningService, resourceOpenerGetter,
		nil, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pi, tc.DeepEquals, &caasapplicationprovisioner.ProvisioningInfo{
		ImageDetails: coreresource.DockerImageDetails{
			RegistryPath: "test-repo/jujud-operator:2.9.99",
			ImageRepoDetails: coreresource.ImageRepoDetails{
				Repository:    "test-repo",
				ServerAddress: "registry.com",
			},
		},
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "22.04",
				Risk:  corebase.Stable,
			},
		},
		Version:              semversion.MustParse("2.9.99"),
		CharmModifiedVersion: 123,
		APIAddresses:         []string{"1.2.3.1", "1.2.3.2", "1.2.3.3"},
		CACert:               "CACERT",
		Tags: map[string]string{
			"tag": "tag-value",
		},
		Trust:       true,
		Scale:       10,
		Constraints: constraints.MustParse("mem=1G"),
		Devices:     []devices.KubernetesDeviceParams{},
		CharmMeta:   chMeta,
		Images: map[string]coreresource.DockerImageDetails{
			"mysql-image": {
				RegistryPath: "mysql/ubuntu:latest-22.04",
			},
			"rootless-image": {
				RegistryPath: "rootless:foo-bar",
			},
		},
		FilesystemTemplates: fsTemplates,
		StorageResourceTags: storageResourceTags,
	})
}
