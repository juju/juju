// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/caasapplication"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&CAASApplicationSuite{})

type CAASApplicationSuite struct {
	testhelpers.IsolationSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     *caasapplication.Facade

	modelUUID               coremodel.UUID
	controllerConfigService *caasapplication.MockControllerConfigService
	applicationService      *caasapplication.MockApplicationService
	modelAgentService       *caasapplication.MockModelAgentService

	controllerState *caasapplication.MockControllerState
}

func (s *CAASApplicationSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })
}

func (s *CAASApplicationSuite) setupMocks(c *tc.C, authTag string) *gomock.Controller {
	ctrl := gomock.NewController(c)

	tag, err := names.ParseTag(authTag)
	c.Assert(err, tc.ErrorIsNil)
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: tag,
	}

	s.modelUUID = modeltesting.GenModelUUID(c)

	s.controllerConfigService = caasapplication.NewMockControllerConfigService(ctrl)
	s.applicationService = caasapplication.NewMockApplicationService(ctrl)
	s.modelAgentService = caasapplication.NewMockModelAgentService(ctrl)
	s.controllerState = caasapplication.NewMockControllerState(ctrl)

	s.facade = caasapplication.NewFacade(s.resources, s.authorizer, s.controllerState,
		coretesting.ControllerTag.Id(), s.modelUUID,
		s.controllerConfigService, s.applicationService, s.modelAgentService, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

func (s *CAASApplicationSuite) TestUnitIntroductionMissingName(c *tc.C) {
	defer s.setupMocks(c, "application-gitlab").Finish()

	result, err := s.facade.UnitIntroduction(c.Context(), params.CAASUnitIntroductionArgs{
		PodUUID: "pod-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CAASUnitIntroductionResult{
		Error: &params.Error{Code: "not valid", Message: "pod-name not valid"},
	})
}

func (s *CAASApplicationSuite) TestUnitIntroductionMissingUUID(c *tc.C) {
	defer s.setupMocks(c, "application-gitlab").Finish()

	result, err := s.facade.UnitIntroduction(c.Context(), params.CAASUnitIntroductionArgs{
		PodName: "gitlab-666",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CAASUnitIntroductionResult{
		Error: &params.Error{Code: "not valid", Message: "pod-uuid not valid"},
	})
}

func (s *CAASApplicationSuite) TestUnitIntroduction(c *tc.C) {
	defer s.setupMocks(c, "application-gitlab").Finish()

	controllerCfg := controller.Config{
		controller.CACertKey: coretesting.CACert,
	}
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controllerCfg, nil)

	hp := []network.SpaceHostPort{{
		SpaceAddress: network.SpaceAddress{
			MachineAddress: network.MachineAddress{
				Value: "10.6.6.6",
			},
		},
		NetPort: 17070,
	}}
	s.controllerState.EXPECT().APIHostPortsForAgents(controllerCfg).Return([]network.SpaceHostPorts{hp}, nil)
	vers := semversion.MustParse("6.6.6")
	s.modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(vers, nil)

	s.applicationService.EXPECT().RegisterCAASUnit(gomock.Any(), application.RegisterCAASUnitParams{
		ApplicationName: "gitlab",
		ProviderID:      "gitlab-666",
	}).Return("gitlab/666", "secret", nil)

	expectedConf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: "/var/lib/juju",
				LogDir:  "/var/log/juju",
			},
			Tag:               names.NewUnitTag("gitlab/666"),
			Controller:        names.NewControllerTag(coretesting.ControllerTag.Id()),
			Model:             names.NewModelTag(s.modelUUID.String()),
			APIAddresses:      []string{"10.6.6.6:17070"},
			CACert:            coretesting.CACert,
			Password:          "secret",
			UpgradedToVersion: vers,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	confBytes, err := expectedConf.Render()
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.facade.UnitIntroduction(c.Context(), params.CAASUnitIntroductionArgs{
		PodName: "gitlab-666",
		PodUUID: "pod-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CAASUnitIntroductionResult{
		Result: &params.CAASUnitIntroduction{
			UnitName:  "gitlab/666",
			AgentConf: confBytes,
		},
	})
}

func (s *CAASApplicationSuite) TestUnitIntroductionPermissionDenied(c *tc.C) {
	defer s.setupMocks(c, "unit-gitlab-666").Finish()

	_, err := s.facade.UnitIntroduction(c.Context(), params.CAASUnitIntroductionArgs{
		PodName: "gitlab-666",
		PodUUID: "pod-uuid",
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *CAASApplicationSuite) TestUnitIntroductionApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c, "application-gitlab").Finish()

	s.applicationService.EXPECT().RegisterCAASUnit(gomock.Any(), application.RegisterCAASUnitParams{
		ApplicationName: "gitlab",
		ProviderID:      "gitlab-666",
	}).Return("", "", applicationerrors.ApplicationNotFound)
	result, err := s.facade.UnitIntroduction(c.Context(), params.CAASUnitIntroductionArgs{
		PodName: "gitlab-666",
		PodUUID: "pod-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CAASUnitIntroductionResult{
		Error: &params.Error{Code: "not found", Message: "application gitlab not found"},
	})
}

func (s *CAASApplicationSuite) TestUnitIntroductionApplicationNotAlive(c *tc.C) {
	defer s.setupMocks(c, "application-gitlab").Finish()

	s.applicationService.EXPECT().RegisterCAASUnit(gomock.Any(), application.RegisterCAASUnitParams{
		ApplicationName: "gitlab",
		ProviderID:      "gitlab-666",
	}).Return("", "", applicationerrors.ApplicationNotAlive)
	result, err := s.facade.UnitIntroduction(c.Context(), params.CAASUnitIntroductionArgs{
		PodName: "gitlab-666",
		PodUUID: "pod-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CAASUnitIntroductionResult{
		Error: &params.Error{Code: "not provisioned", Message: "application gitlab not provisioned"},
	})
}

func (s *CAASApplicationSuite) TestUnitIntroductionUnitNotAssigned(c *tc.C) {
	defer s.setupMocks(c, "application-gitlab").Finish()

	s.applicationService.EXPECT().RegisterCAASUnit(gomock.Any(), application.RegisterCAASUnitParams{
		ApplicationName: "gitlab",
		ProviderID:      "gitlab-666",
	}).Return("", "", applicationerrors.UnitNotAssigned)
	result, err := s.facade.UnitIntroduction(c.Context(), params.CAASUnitIntroductionArgs{
		PodName: "gitlab-666",
		PodUUID: "pod-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CAASUnitIntroductionResult{
		Error: &params.Error{Code: "not assigned", Message: "unit for pod gitlab-666 not assigned"},
	})
}

func (s *CAASApplicationSuite) TestUnitIntroductionUnitAlreadyExists(c *tc.C) {
	defer s.setupMocks(c, "application-gitlab").Finish()

	s.applicationService.EXPECT().RegisterCAASUnit(gomock.Any(), application.RegisterCAASUnitParams{
		ApplicationName: "gitlab",
		ProviderID:      "gitlab-666",
	}).Return("", "", applicationerrors.UnitAlreadyExists)
	result, err := s.facade.UnitIntroduction(c.Context(), params.CAASUnitIntroductionArgs{
		PodName: "gitlab-666",
		PodUUID: "pod-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CAASUnitIntroductionResult{
		Error: &params.Error{Code: "already exists", Message: "unit for pod gitlab-666 already exists"},
	})
}

func (s *CAASApplicationSuite) TestUnitTerminating(c *tc.C) {
	defer s.setupMocks(c, "unit-gitlab-666").Finish()

	s.applicationService.EXPECT().CAASUnitTerminating(gomock.Any(), "gitlab/666").Return(true, nil)

	result, err := s.facade.UnitTerminating(c.Context(), params.Entity{
		Tag: "unit-gitlab-666",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CAASUnitTerminationResult{
		WillRestart: true,
	})
}

func (s *CAASApplicationSuite) TestUnitTerminatingNotFound(c *tc.C) {
	defer s.setupMocks(c, "unit-gitlab-666").Finish()

	s.applicationService.EXPECT().CAASUnitTerminating(gomock.Any(), "gitlab/666").Return(false, applicationerrors.UnitNotFound)

	result, err := s.facade.UnitTerminating(c.Context(), params.Entity{
		Tag: "unit-gitlab-666",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CAASUnitTerminationResult{
		Error: &params.Error{
			Code:    "not found",
			Message: "unit gitlab/666 not found",
		},
	})
}

func (s *CAASApplicationSuite) TestUnitTerminatingPermissionDenied(c *tc.C) {
	defer s.setupMocks(c, "unit-gitlab-666").Finish()

	_, err := s.facade.UnitTerminating(c.Context(), params.Entity{
		Tag: "unit-mysql-666",
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
