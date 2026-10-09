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
	applicationerrors "github.com/juju/juju/domain/application/errors"
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

func (s *OpsSuite) TestUpdateStateMissingServiceClearsAddresses(c *tc.C) {
	ctrl := gomock.NewController(c)
	app := caasmocks.NewMockApplication(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	appUUID := tc.Must0(c, application.NewUUID)
	app.EXPECT().Service().Return(nil, caas.ServiceNotFound)
	applicationService.EXPECT().ClearK8sServiceAddresses(gomock.Any(), appUUID).Return(nil)
	applicationService.EXPECT().GetAllUnitK8sPodIDsForApplication(gomock.Any(), appUUID).Return(nil, nil)
	app.EXPECT().Units().Return(nil, nil)
	_, err := caasapplicationprovisioner.AppOps.UpdateState(c.Context(), "controller", appUUID, app, nil,
		nil, applicationService, nil, testclock.NewClock(time.Time{}), s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestUpdateStateServiceReadFailurePreservesAddresses(c *tc.C) {
	ctrl := gomock.NewController(c)
	app := caasmocks.NewMockApplication(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	expectedErr := errors.New("provider unavailable")
	app.EXPECT().Service().Return(nil, expectedErr)
	_, err := caasapplicationprovisioner.AppOps.UpdateState(c.Context(), "controller", tc.Must0(c, application.NewUUID), app, nil,
		nil, applicationService, nil, testclock.NewClock(time.Time{}), s.logger)
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *OpsSuite) TestUpdateStateMissingWorkloadPreservesServiceAddresses(c *tc.C) {
	ctrl := gomock.NewController(c)
	app := caasmocks.NewMockApplication(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	appUUID := tc.Must0(c, application.NewUUID)
	app.EXPECT().Service().Return(nil, errors.NotFoundf("statefulset"))
	applicationService.EXPECT().GetAllUnitK8sPodIDsForApplication(gomock.Any(), appUUID).Return(nil, nil)
	app.EXPECT().Units().Return(nil, nil)
	_, err := caasapplicationprovisioner.AppOps.UpdateState(c.Context(), "controller", appUUID, app, nil,
		nil, applicationService, nil, testclock.NewClock(time.Time{}), s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestUpdateStateClearServiceAddressesFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	app := caasmocks.NewMockApplication(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	appUUID := tc.Must0(c, application.NewUUID)
	expectedErr := errors.New("state unavailable")
	app.EXPECT().Service().Return(nil, caas.ServiceNotFound)
	applicationService.EXPECT().ClearK8sServiceAddresses(gomock.Any(), appUUID).Return(expectedErr)
	_, err := caasapplicationprovisioner.AppOps.UpdateState(c.Context(), "controller", appUUID, app, nil,
		nil, applicationService, nil, testclock.NewClock(time.Time{}), s.logger)
	c.Assert(err, tc.ErrorIs, expectedErr)
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

func (s *OpsSuite) TestEnsureScaleSchedulesDepartures(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	units := map[unit.Name]life.Value{
		"test/0": life.Dead,
		"test/1": life.Dying,
		"test/2": life.Alive,
		"test/3": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).Return(units, nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), []string{"test/1"}).Return(nil),
		facade.EXPECT().RemoveUnit(gomock.Any(), "test/0").Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appID, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorMatches, "try again")
}

func (s *OpsSuite) TestEnsureScaleReservesBeforeStartingPods(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	units := map[unit.Name]life.Value{
		"test/2": life.Alive,
		"test/3": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).Return(units, nil),
		applicationService.EXPECT().ReserveCAASUnits(gomock.Any(), "test", 2, 2).Return(nil),
		app.EXPECT().State().Return(caas.ApplicationState{DesiredReplicas: 1, StartOrdinal: 2}, nil),
		facade.EXPECT().FilesystemProvisioningInfo(gomock.Any(), "test").Return(provisionertypes.FilesystemProvisioningInfo{}, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any(), appID.String()[:6]).Return(nil),
		app.EXPECT().ScaleRange(gomock.Any(), 2, 2).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appID, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorMatches, "try again")
}

func (s *OpsSuite) TestEnsureScaleDoesNotStartPodsWhenReservationFails(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	units := map[unit.Name]life.Value{"test/0": life.Alive}
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).Return(units, nil),
		applicationService.EXPECT().ReserveCAASUnits(gomock.Any(), "test", 0, 1).Return(errors.New("reservation failed")),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appID, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorMatches, ".*reservation failed")
}

func (s *OpsSuite) TestEnsureScaleWaitsForPodRegistration(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	units := map[unit.Name]life.Value{
		"test/0": life.Alive, "test/1": life.Alive, "test/2": life.Alive,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).Return(units, nil),
		applicationService.EXPECT().ReserveCAASUnits(gomock.Any(), "test", 0, 3).Return(nil),
		app.EXPECT().State().Return(caas.ApplicationState{
			DesiredReplicas: 3, Replicas: []string{"test-0", "test-1", "test-2"},
		}, nil),
		applicationService.EXPECT().GetAllUnitK8sPodIDsForApplication(gomock.Any(), appID).
			Return(map[unit.Name]string{"test/0": "test-0", "test/2": "test-2"}, nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appID, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorMatches, "try again")
}

func (s *OpsSuite) TestEnsureScaleUsesSequenceAtZero(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).
			Return(map[unit.Name]life.Value{}, nil),
		applicationService.EXPECT().GetApplicationUnitSequence(gomock.Any(), "test").
			Return(uint64(2), true, nil),
		applicationService.EXPECT().ReserveCAASUnits(gomock.Any(), "test", 3, 0).Return(nil),
		app.EXPECT().State().Return(caas.ApplicationState{DesiredReplicas: 1, StartOrdinal: 2}, nil),
		facade.EXPECT().FilesystemProvisioningInfo(gomock.Any(), "test").Return(provisionertypes.FilesystemProvisioningInfo{}, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any(), appID.String()[:6]).Return(nil),
		app.EXPECT().ScaleRange(gomock.Any(), 0, 3).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appID, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorMatches, "try again")
}

func (s *OpsSuite) TestEnsureScaleRejectsNonContiguousIntent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	units := map[unit.Name]life.Value{"test/0": life.Alive, "test/2": life.Alive}
	applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).Return(units, nil)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appID, app,
		life.Alive, facade, applicationService, nil, s.logger)
	c.Assert(err, tc.ErrorMatches, "intended unit range is not contiguous: .*")
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
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).Return(nil, nil),
		applicationService.EXPECT().GetApplicationUnitSequence(gomock.Any(), "test").Return(uint64(0), false, nil),
		app.EXPECT().State().Return(caas.ApplicationState{}, nil),
	)

	err := caasapplicationprovisioner.AppOps.AppDying(c.Context(), "test", appUUID, app,
		life.Dying, facade, applicationService, statusService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestAppDyingApplicationNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	// The application rows are gone from state before the worker can scale
	// the application down, for example after a forced removal. AppDying
	// must not fail so the caller can still run the dead cleanup path and
	// delete the k8s resources.
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).
			Return(nil, applicationerrors.ApplicationNotFound),
		applicationService.EXPECT().GetApplicationUnitSequence(gomock.Any(), "test").
			Return(0, false, applicationerrors.ApplicationNotFound),
		app.EXPECT().State().Return(caas.ApplicationState{}, applicationerrors.ApplicationNotFound),
	)

	err := caasapplicationprovisioner.AppOps.AppDying(c.Context(), "test", appUUID, app,
		life.Dying, facade, applicationService, statusService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestAppDyingWithDeadUnit(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appUUID).
			Return(map[unit.Name]life.Value{"test/0": life.Dead}, nil),
		applicationService.EXPECT().GetApplicationUnitSequence(gomock.Any(), "test").Return(0, true, nil),
		facade.EXPECT().RemoveUnit(gomock.Any(), "test/0").Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.AppDying(c.Context(), "test", appUUID, app,
		life.Dying, facade, applicationService, statusService, s.logger)
	c.Assert(err, tc.ErrorMatches, "cannot scale dying application to 0: try again")
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
