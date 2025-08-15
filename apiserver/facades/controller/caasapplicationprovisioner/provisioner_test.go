// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/removal"
	envconfig "github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestCAASApplicationProvisionerSuite(t *testing.T) {
	tc.Run(t, &CAASApplicationProvisionerSuite{})
}

type CAASApplicationProvisionerSuite struct {
	coretesting.BaseSuite
	clock clock.Clock

	watcherRegistry         *facademocks.MockWatcherRegistry
	authorizer              *apiservertesting.FakeAuthorizer
	api                     *caasapplicationprovisioner.API
	applicationService      *MockApplicationService
	controllerConfigService *MockControllerConfigService
	controllerNodeService   *MockControllerNodeService
	modelConfigService      *MockModelConfigService
	modelInfoService        *MockModelInfoService
	statusService           *MockStatusService
	removalService          *MockRemovalService
}

func (s *CAASApplicationProvisionerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.PatchValue(&jujuversion.OfficialBuild, 0)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.clock = testclock.NewClock(time.Now())
}

func (s *CAASApplicationProvisionerSuite) setupAPI(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	api, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(
		s.authorizer,
		caasapplicationprovisioner.Services{
			ApplicationService:      s.applicationService,
			ControllerConfigService: s.controllerConfigService,
			ControllerNodeService:   s.controllerNodeService,
			ModelConfigService:      s.modelConfigService,
			ModelInfoService:        s.modelInfoService,
			StatusService:           s.statusService,
			RemovalService:          s.removalService,
		},
		s.clock,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry)

	c.Assert(err, tc.ErrorIsNil)
	s.api = api

	c.Cleanup(func() {
		s.api = nil
		s.applicationService = nil
		s.controllerNodeService = nil
		s.modelConfigService = nil
		s.modelInfoService = nil
		s.statusService = nil
		s.watcherRegistry = nil
	})

	return ctrl
}

func (s *CAASApplicationProvisionerSuite) TestPermission(c *tc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(
		s.authorizer,
		caasapplicationprovisioner.Services{},
		s.clock,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *CAASApplicationProvisionerSuite) TestProvisioningInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	locator := applicationcharm.CharmLocator{
		Name:     "gitlab",
		Source:   applicationcharm.CharmHubSource,
		Revision: -1,
	}
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "gitlab").Return(locator, nil)
	s.applicationService.EXPECT().IsCharmAvailable(gomock.Any(), locator).Return(true, nil)

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(s.fakeModelConfig())
	addrs := []string{"10.0.0.1:1"}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)

	modelInfo := model.ModelInfo{
		UUID: model.UUID(coretesting.ModelTag.Id()),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)
	s.modelInfoService.EXPECT().ResolveConstraints(gomock.Any(), constraints.Value{}).Return(constraints.Value{}, nil)

	s.applicationService.EXPECT().GetApplicationScale(gomock.Any(), "gitlab").Return(3, nil)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return(coreapplication.ID("deadbeef"), nil)
	s.applicationService.EXPECT().GetApplicationConstraints(gomock.Any(), coreapplication.ID("deadbeef")).Return(constraints.Value{}, nil)
	s.applicationService.EXPECT().GetDeviceConstraints(gomock.Any(), "gitlab").Return(map[string]devices.Constraints{}, nil)
	s.applicationService.EXPECT().GetApplicationCharmOrigin(gomock.Any(), "gitlab").Return(application.CharmOrigin{
		Platform: deployment.Platform{
			Channel: "stable",
			OSType:  deployment.Ubuntu,
		},
	}, nil)
	s.applicationService.EXPECT().GetCharmModifiedVersion(gomock.Any(), coreapplication.ID("deadbeef")).Return(10, nil)
	s.applicationService.EXPECT().GetApplicationTrustSetting(gomock.Any(), "gitlab").Return(true, nil)

	result, err := s.api.ProvisioningInfo(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
	c.Assert(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Results[0].CACert`, tc.Ignore)
	c.Assert(result, mc, params.CAASApplicationProvisioningInfoResults{
		Results: []params.CAASApplicationProvisioningInfo{{
			ImageRepo:    params.DockerImageInfo{RegistryPath: "ghcr.io/juju/jujud-operator:2.6-beta3.666"},
			Version:      semversion.MustParse("2.6-beta3.666"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id(),
			},
			CharmModifiedVersion: 10,
			Scale:                3,
			Trust:                true,
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "stable",
			},
		}},
	})
}

func (s *CAASApplicationProvisionerSuite) TestProvisioningInfoPendingCharmError(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	locator := applicationcharm.CharmLocator{
		Name:     "gitlab",
		Source:   applicationcharm.CharmHubSource,
		Revision: -1,
	}
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "gitlab").Return(locator, nil)
	s.applicationService.EXPECT().IsCharmAvailable(gomock.Any(), locator).Return(false, nil)

	result, err := s.api.ProvisioningInfo(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `charm for application "gitlab" not provisioned`)
}

func (s *CAASApplicationProvisionerSuite) TestWatchProvisioningInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	appChanged := make(chan struct{}, 1)
	portsChanged := make(chan struct{}, 1)
	modelConfigChanged := make(chan []string, 1)
	controllerConfigChanged := make(chan []string, 1)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(portsChanged), nil)
	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(watchertest.NewMockStringsWatcher(controllerConfigChanged), nil)
	s.modelConfigService.EXPECT().Watch(gomock.Any()).Return(watchertest.NewMockStringsWatcher(modelConfigChanged), nil)
	s.applicationService.EXPECT().WatchApplication(gomock.Any(), "gitlab").Return(watchertest.NewMockNotifyWatcher(appChanged), nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("42", nil)

	appChanged <- struct{}{}
	portsChanged <- struct{}{}
	modelConfigChanged <- []string{}
	controllerConfigChanged <- []string{}

	results, err := s.api.WatchProvisioningInfo(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].NotifyWatcherId, tc.Equals, "42")
}

func (s *CAASApplicationProvisionerSuite) fakeModelConfig() (*envconfig.Config, error) {
	attrs := coretesting.FakeConfig()
	attrs["agent-version"] = "2.6-beta3.666"
	return envconfig.New(envconfig.UseDefaults, attrs)
}

func (s *CAASApplicationProvisionerSuite) TestRemove(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("gitlab/0")).Return(coreunit.UUID("unit-uuid"), nil)
	s.removalService.EXPECT().MarkUnitAsDead(gomock.Any(), coreunit.UUID("unit-uuid")).Return(nil)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), coreunit.UUID("unit-uuid"), false, time.Duration(0)).Return(removal.UUID("removal-uuid"), nil)

	result, err := s.api.Remove(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "unit-gitlab-0",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{
		Error: nil,
	}}})
}

func (s *CAASApplicationProvisionerSuite) TestDestroyUnits(c *tc.C) {
	defer s.setupAPI(c).Finish()

	// Arrange
	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), unitUUID, false, time.Duration(0)).Return(removal.UUID(""), nil)

	// Act
	res, err := s.api.DestroyUnits(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *CAASApplicationProvisionerSuite) TestDestroyUnitsForce(c *tc.C) {
	defer s.setupAPI(c).Finish()

	d := time.Hour

	// Arrange
	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), unitUUID, true, time.Hour).Return(removal.UUID(""), nil)

	// Act
	res, err := s.api.DestroyUnits(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag(unitName.String()).String(),
			Force:   true,
			MaxWait: &d,
		}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *CAASApplicationProvisionerSuite) TestDestroyUnitsNotFound(c *tc.C) {
	defer s.setupAPI(c).Finish()

	// Arrange
	unitName := coreunit.Name("foo/0")
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(coreunit.UUID(""), applicationerrors.UnitNotFound)

	// Act
	res, err := s.api.DestroyUnits(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}
