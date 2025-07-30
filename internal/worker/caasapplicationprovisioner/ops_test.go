// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

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
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
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

	appId, _ := application.NewID()
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
	appStatus := status.StatusInfo{
		Status:  status.Active,
		Message: "nice message",
		Data: map[string]any{
			"nice": "data",
		},
		Since: &now,
	}
	cloudContainerIDs := map[unit.Name]string{
		"test/0": "a",
		"test/1": "b",
	}

	unit0Update := applicationservice.UpdateCAASUnitParams{
		ProviderID: ptr("a"),
		Address:    ptr("1.2.3.5"),
		Ports:      ptr([]string{"80", "443"}),
	}

	gomock.InOrder(
		app.EXPECT().Service().Return(service, nil),
		applicationService.EXPECT().UpdateCloudService(gomock.Any(), "test", "provider-id", network.ProviderAddresses{{
			MachineAddress: network.NewMachineAddress("1.2.3.4"),
			SpaceName:      "space-name",
		}}).Return(nil),
		statusService.EXPECT().SetApplicationStatus(gomock.Any(), "test", appStatus).Return(nil),
		applicationService.EXPECT().GetAllUnitCloudContainerIDsForApplication(gomock.Any(), appId).Return(cloudContainerIDs, nil),
		app.EXPECT().Units().Return(units, nil),
		applicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unit.Name("test/0"), gomock.Any()).DoAndReturn(func(_ context.Context, _ unit.Name, args applicationservice.UpdateCAASUnitParams) error {
			c.Check(args, tc.DeepEquals, unit0Update)
			return nil
		}),
		broker.EXPECT().AnnotateUnit(gomock.Any(), "test", "a", names.NewUnitTag("test/0")).Return(nil),
	)

	lastReportedStatus := caasapplicationprovisioner.UpdateStatusState{
		"test/1": {
			ProviderID: ptr("b"),
			Address:    ptr("1.2.3.6"),
			Ports:      ptr([]string{"80", "443"}),
			AgentStatus: &status.StatusInfo{
				Status:  status.Allocating,
				Message: "same",
			},
			CloudContainerStatus: &status.StatusInfo{
				Status:  status.Waiting,
				Message: "same",
			},
		},
	}
	currentReportedStatus, err := caasapplicationprovisioner.AppOps.UpdateState(c.Context(), "test", appId, app, lastReportedStatus, broker, applicationService, statusService, clk, s.logger)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(currentReportedStatus, tc.DeepEquals, caasapplicationprovisioner.UpdateStatusState{
		"test/0": {
			ProviderID: ptr("a"),
			Address:    ptr("1.2.3.5"),
			Ports:      ptr([]string{"80", "443"}),
		},
		"test/1": {
			ProviderID: ptr("b"),
			Address:    ptr("1.2.3.6"),
			Ports:      ptr([]string{"80", "443"}),
			AgentStatus: &status.StatusInfo{
				Status:  status.Allocating,
				Message: "same",
			},
			CloudContainerStatus: &status.StatusInfo{
				Status:  status.Waiting,
				Message: "same",
			},
		},
	})
}

func (s *OpsSuite) TestRefreshApplicationStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appLife := life.Alive
	appId, _ := application.NewID()
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

	err := caasapplicationprovisioner.AppOps.RefreshApplicationStatus(c.Context(), "test", appId, app, appLife, statusService, clk, s.logger)
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
	statusService := mocks.NewMockStatusService(ctrl)

	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)
	password := "123456789"
	lastApplied := caas.ApplicationConfig{}

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
					Uid:      ptr(5000),
					Gid:      ptr(5001),
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
			StorageName:  "data",
			Count:        1,
			MaxCount:     1,
			SizeMiB:      100,
			ProviderType: "kubernetes",
			ReadOnly:     false,
			Location:     "/charm-defined-location",
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
				Uid: ptr(5000),
				Gid: ptr(5001),
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
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				ReadOnly: false,
				Path:     "/charm-defined-location",
			},
		}},
		Devices:      []devices.KubernetesDeviceParams{},
		Trust:        true,
		InitialScale: 10,
		CharmUser:    caas.RunAsDefault,
	}
	gomock.InOrder(
		app.EXPECT().Exists().Return(ds, nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		app.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
			c.Check(config, tc.DeepEquals, ensureParams)
			return nil
		}),
	)

	err := caasapplicationprovisioner.AppOps.AppAlive(c.Context(), "test", app, password, &lastApplied, &pi, statusService, clk, s.logger)
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

	appId, _ := application.NewID()
	app := caasmocks.NewMockApplication(ctrl)
	broker := mocks.NewMockCAASBroker(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	statusService := mocks.NewMockStatusService(ctrl)

	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	gomock.InOrder(
		app.EXPECT().Delete().Return(nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		app.EXPECT().Service().Return(nil, errors.NotFound),
		applicationService.EXPECT().GetAllUnitCloudContainerIDsForApplication(gomock.Any(), appId).Return(nil, nil),
		app.EXPECT().Units().Return(nil, nil),
	)

	err := caasapplicationprovisioner.AppOps.AppDead(c.Context(), "test", appId, app, broker, applicationService, statusService, clk, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpsSuite) TestProvisioningInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appId, _ := application.NewID()
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	storageProvisioningService := mocks.NewMockStorageProvisioningService(ctrl)
	applicationService := mocks.NewMockApplicationService(ctrl)
	resourceOpenerGetter := mocks.NewMockResourceOpenerGetter(ctrl)
	ro := mocks.NewMockOpener(ctrl)
	resourceOpenerGetter.EXPECT().ResourceOpenerForApplication(gomock.Any(), appId, "test").Return(ro, nil)

	facadePi := api.ProvisioningInfo{
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
		MaxCount:     1,
		SizeMiB:      100,
		ProviderType: "kubernetes",
		ReadOnly:     false,
		Location:     "/charm-defined-location",
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
				Uid:      ptr(5000),
				Gid:      ptr(5001),
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
	ch := charm.NewCharmBase(chMeta, nil, nil, nil, nil)
	applicationService.EXPECT().GetCharmByApplicationID(gomock.Any(), appId).Return(ch, applicationcharm.CharmLocator{}, nil)

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

	pi, err := caasapplicationprovisioner.AppOps.ProvisioningInfo(c.Context(), "test", appId, facade, storageProvisioningService, applicationService, resourceOpenerGetter, nil, s.logger)
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

func ptr[T any](i T) *T {
	return &i
}
