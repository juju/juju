// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"context"
	"testing"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	charmscommon "github.com/juju/juju/api/common/charms"
	api "github.com/juju/juju/api/controller/caasapplicationprovisioner"
	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/application"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
	"github.com/juju/juju/rpc/params"
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

func (s *OpsSuite) TestCheckCharmFormatV1(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfoV1 := &charmscommon.CharmInfo{
		Meta: &charm.Meta{Name: "test"},
	}

	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	// Wait till charm is v2
	facade.EXPECT().ApplicationCharmInfo(gomock.Any(), "test").Return(charmInfoV1, nil)

	isOk, err := caasapplicationprovisioner.AppOps.CheckCharmFormat(c.Context(), "test", facade, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isOk, tc.IsFalse)
}

func (s *OpsSuite) TestCheckCharmFormatV2(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfoV2 := &charmscommon.CharmInfo{
		Meta:     &charm.Meta{Name: "test"},
		Manifest: &charm.Manifest{Bases: []charm.Base{{}}},
	}

	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	// Wait till charm is v2
	facade.EXPECT().ApplicationCharmInfo(gomock.Any(), "test").Return(charmInfoV2, nil)

	isOk, err := caasapplicationprovisioner.AppOps.CheckCharmFormat(c.Context(), "test", facade, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isOk, tc.IsTrue)
}

func (s *OpsSuite) TestCheckCharmFormatNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	facade.EXPECT().ApplicationCharmInfo(gomock.Any(), "test").DoAndReturn(func(_ context.Context, appName string) (*charmscommon.CharmInfo, error) {
		return nil, errors.NotFoundf("test charm")
	})

	isOk, err := caasapplicationprovisioner.AppOps.CheckCharmFormat(c.Context(), "test", facade, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isOk, tc.IsFalse)
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

	app := caasmocks.NewMockApplication(ctrl)
	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	appTag := names.NewApplicationTag("test").String()
	service := &caas.Service{
		Id: "provider-id",
		Status: status.StatusInfo{
			Status:  status.Active,
			Message: "nice message",
			Data: map[string]interface{}{
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
			Status:  status.Active,
			Message: "different",
		},
		FilesystemInfo: []caas.FilesystemInfo{{
			StorageName:  "s",
			FilesystemId: "fsid",
			Volume: caas.VolumeInfo{
				VolumeId: "vid",
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
	updateUnitsArg := params.UpdateApplicationUnits{
		ApplicationTag: appTag,
		Status: params.EntityStatus{
			Status: status.Active,
			Info:   "nice message",
			Data: map[string]interface{}{
				"nice": "data",
			},
		},
		Scale: nil,
		Units: []params.ApplicationUnitParams{{
			ProviderId: "a",
			Address:    "1.2.3.5",
			Ports:      []string{"80", "443"},
			Stateful:   true,
			Status:     "active",
			Info:       "different",
			FilesystemInfo: []params.KubernetesFilesystemInfo{{
				StorageName:  "s",
				FilesystemId: "fsid",
				Volume: params.KubernetesVolumeInfo{
					VolumeId: "vid",
				},
			}},
		}, {
			ProviderId: "b",
			Address:    "1.2.3.6",
			Ports:      []string{"80", "443"},
			Stateful:   true,
			Status:     "unknown",
		}},
	}
	appUnitInfo := &params.UpdateApplicationUnitsInfo{
		Units: []params.ApplicationUnitInfo{{
			UnitTag:    "unit-test-0",
			ProviderId: "a",
		}, {
			UnitTag:    "unit-test-1",
			ProviderId: "b",
		}},
	}
	gomock.InOrder(
		app.EXPECT().Service().Return(service, nil),
		applicationService.EXPECT().UpdateCloudService(gomock.Any(), "test", "provider-id", network.ProviderAddresses{{
			MachineAddress: network.NewMachineAddress("1.2.3.4"),
			SpaceName:      "space-name",
		}}).Return(nil),
		app.EXPECT().Units().Return(units, nil),
		facade.EXPECT().UpdateUnits(gomock.Any(), updateUnitsArg).Return(appUnitInfo, nil),
		broker.EXPECT().AnnotateUnit(gomock.Any(), "test", "a", names.NewUnitTag("test/0")).Return(nil),
		broker.EXPECT().AnnotateUnit(gomock.Any(), "test", "b", names.NewUnitTag("test/1")).Return(nil),
	)

	lastReportedStatus := map[string]status.StatusInfo{
		"b": {
			Status:  status.Allocating,
			Message: "same",
		},
	}
	currentReportedStatus, err := caasapplicationprovisioner.AppOps.UpdateState(c.Context(), "test", app, lastReportedStatus, broker, facade, applicationService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(currentReportedStatus, tc.DeepEquals, map[string]status.StatusInfo{
		"a": {Status: "active", Message: "different"},
		"b": {Status: "allocating", Message: "same"},
	})
}

func (s *OpsSuite) TestRefreshApplicationStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appLife := life.Alive
	appId, _ := application.NewID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
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
		statusService.EXPECT().SetApplicationStatus(gomock.Any(), "test", gomock.Any()).DoAndReturn(func(ctx context.Context, name string, si status.StatusInfo) error {
			mc := tc.NewMultiChecker()
			mc.AddExpr("_.Since", tc.NotNil)
			c.Check(si, mc, status.StatusInfo{
				Status:  status.Waiting,
				Message: "waiting for units to settle down",
			})
			return nil
		}),
	)

	err := caasapplicationprovisioner.AppOps.RefreshApplicationStatus(c.Context(), "test", appId, app, appLife, facade, statusService, clk, s.logger)
	c.Assert(err, tc.ErrorIsNil)
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

	appId, _ := application.NewID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Dead,
	}
	ps := applicationservice.ScalingState{
		Scaling:     true,
		ScaleTarget: 1,
	}
	appState := caas.ApplicationState{
		Replicas: []string{
			"a",
		},
	}
	gomock.InOrder(
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
		app.EXPECT().Scale(1).Return(nil),
		app.EXPECT().State().Return(appState, nil),
		facade.EXPECT().RemoveUnit(gomock.Any(), "test/1").Return(nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 0, false).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale(c.Context(), "test", appId, app, facade, applicationService, statusService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAlive(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	units := map[unit.Name]life.Value{
		"test/0": life.Alive,
		"test/1": life.Alive,
		"test/2": life.Dying,
		"test/3": life.Dead,
	}
	unitsToDestroy := []string{"test/1"}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(1, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 1, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), unitsToDestroy).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app, life.Alive, facade, applicationService, statusService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveRetry(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

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
	unitsToDestroy := []string{"test/1"}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScale(gomock.Any(), "test").Return(10, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(ps, nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
		facade.EXPECT().DestroyUnits(gomock.Any(), unitsToDestroy).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app, life.Alive, facade, applicationService, statusService, s.logger)
	c.Assert(err, tc.ErrorMatches, `try again`)
}

func (s *OpsSuite) TestEnsureScaleDyingDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	units := map[unit.Name]life.Value{
		"test/0": life.Dying,
		"test/1": life.Dead,
	}
	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 0, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(units, nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale(c.Context(), "test", appId, app, life.Dead, facade, applicationService, statusService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestAppAlive(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)
	password := "123456789"
	lastApplied := caas.ApplicationConfig{}

	pi := api.ProvisioningInfo{
		CharmURL: charm.MustParseURL("ch:my-app"),
		ImageDetails: resource.DockerImageDetails{
			RegistryPath: "test-repo/jujud-operator:2.9.99",
			ImageRepoDetails: resource.ImageRepoDetails{
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
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
		}},
		Devices: []devices.KubernetesDeviceParams{},
	}
	charmInfo := charmscommon.CharmInfo{
		Meta: &charm.Meta{
			Containers: map[string]charm.Container{
				"mysql": {
					Resource: "mysql-image",
					Mounts: []charm.Mount{{
						Storage:  "data",
						Location: "/data",
					}},
				},
				"rootless": {
					Resource: "rootless-image",
					Uid:      intPtr(5000),
					Gid:      intPtr(5001),
				},
			},
		},
	}
	ds := caas.DeploymentState{
		Exists:      true,
		Terminating: true,
	}
	oci := map[string]resource.DockerImageDetails{
		"mysql-image": {
			RegistryPath: "mysql/ubuntu:latest-22.04",
		},
		"rootless-image": {
			RegistryPath: "rootless:foo-bar",
		},
	}
	ensureParams := caas.ApplicationConfig{
		AgentVersion:         semversion.Number{Major: 2, Minor: 9, Patch: 99},
		AgentImagePath:       "test-repo/jujud-operator:2.9.99",
		CharmBaseImagePath:   "test-repo/charm-base:ubuntu-22.04",
		CharmModifiedVersion: 123,
		Containers: map[string]caas.ContainerConfig{
			"mysql": {
				Name: "mysql",
				Image: resource.DockerImageDetails{
					RegistryPath: "mysql/ubuntu:latest-22.04",
				},
				Mounts: []caas.MountConfig{{
					StorageName: "data",
					Path:        "/data",
				}},
			},
			"rootless": {
				Name: "rootless",
				Image: resource.DockerImageDetails{
					RegistryPath: "rootless:foo-bar",
				},
				Uid: intPtr(5000),
				Gid: intPtr(5001),
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
		}},
		Devices:      []devices.KubernetesDeviceParams{},
		Trust:        true,
		InitialScale: 10,
		CharmUser:    caas.RunAsDefault,
	}
	gomock.InOrder(
		facade.EXPECT().ProvisioningInfo(gomock.Any(), "test").Return(pi, nil),
		facade.EXPECT().CharmInfo(gomock.Any(), "ch:my-app").Return(&charmInfo, nil),
		app.EXPECT().Exists().Return(ds, nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		facade.EXPECT().ApplicationOCIResources(gomock.Any(), "test").Return(oci, nil),
		app.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
			c.Check(config, tc.DeepEquals, ensureParams)
			return nil
		}),
	)

	err := caasapplicationprovisioner.AppOps.AppAlive(c.Context(), "test", app, password, &lastApplied, facade, statusService, clk, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestAppDying(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewID()
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	gomock.InOrder(
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 0, true).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(nil, nil),
		app.EXPECT().Scale(0).Return(nil),
		applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "test", 0, false).Return(nil),
		applicationService.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appId).Return(nil, nil),
		applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "test").Return(applicationservice.ScalingState{}, nil),
	)

	err := caasapplicationprovisioner.AppOps.AppDying(c.Context(), "test", appId, app, life.Dying, facade, applicationService, statusService, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestAppDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	broker := mocks.NewMockCAASBroker(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)

	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	appTag := names.NewApplicationTag("test").String()
	updateUnitsArgs := params.UpdateApplicationUnits{
		ApplicationTag: appTag,
	}
	gomock.InOrder(
		app.EXPECT().Delete().Return(nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		app.EXPECT().Service().Return(nil, errors.NotFound),
		app.EXPECT().Units().Return(nil, nil),
		facade.EXPECT().UpdateUnits(gomock.Any(), updateUnitsArgs).Return(nil, nil),
		facade.EXPECT().ClearApplicationResources(gomock.Any(), "test").Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.AppDead(c.Context(), "test", app, broker, facade, applicationService, clk, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func intPtr(i int) *int {
	return &i
}
