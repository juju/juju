// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/rpc/params"
)

type clientSuite struct {
	authorizer            *MockAuthorizer
	controllerNodeService *MockControllerNodeService
	blockCommandService   *MockBlockCommandService
	applicationService    *MockApplicationService
}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) TestStub(c *tc.C) {
	c.Skipf(`This suite is missing tests for the following scenarios:

- Test enable HA with CAAS (currently not supported)
 `)
}

func (s *clientSuite) TestEnableHAPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(errors.New("boom"))

	api := HighAvailabilityAPI{
		authorizer: s.authorizer,
	}
	_, err := api.EnableHA(c.Context(), params.ControllersSpecs{})
	c.Assert(err, tc.DeepEquals, &params.Error{Message: "permission denied", Code: "unauthorized access"})
}

func (s *clientSuite) TestEnableHANotControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)

	api := HighAvailabilityAPI{
		authorizer:        s.authorizer,
		isControllerModel: false,
	}
	results, err := api.EnableHA(c.Context(), params.ControllersSpecs{
		Specs: []params.ControllersSpec{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.DeepEquals, []params.ControllersChangeResult{{
		Error: &params.Error{Message: "workload models not supported", Code: "not supported"},
	}})
}

func (s *clientSuite) TestEnableHABlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("not allowed", nil)

	api := HighAvailabilityAPI{
		authorizer:          s.authorizer,
		blockCommandService: s.blockCommandService,
		isControllerModel:   true,
	}
	results, err := api.EnableHA(c.Context(), params.ControllersSpecs{
		Specs: []params.ControllersSpec{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.DeepEquals, []params.ControllersChangeResult{{
		Error: &params.Error{Message: "not allowed", Code: "operation is blocked"},
	}})
}

func (s *clientSuite) TestEnableHANumControllersEmptySpec(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.applicationService.EXPECT().AddControllerIAASUnits(gomock.Any(), []string{"0"}, []applicationservice.AddIAASUnitArg{{
		AddUnitArg: applicationservice.AddUnitArg{},
	}, {
		AddUnitArg: applicationservice.AddUnitArg{},
	}, {
		AddUnitArg: applicationservice.AddUnitArg{},
	}}).Return([]coremachine.Name{"1", "2"}, nil)

	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"0"}, nil)
	s.controllerNodeService.EXPECT().CurateNodes(gomock.Any(), []string{"1", "2"}, nil).Return(nil)

	api := HighAvailabilityAPI{
		authorizer:            s.authorizer,
		blockCommandService:   s.blockCommandService,
		applicationService:    s.applicationService,
		controllerNodeService: s.controllerNodeService,
		isControllerModel:     true,
	}
	results, err := api.EnableHA(c.Context(), params.ControllersSpecs{
		Specs: []params.ControllersSpec{{
			NumControllers: 0,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.DeepEquals, []params.ControllersChangeResult{{
		Result: params.ControllersChanges{
			Added:      []string{"1", "2"},
			Maintained: []string{"0"},
		},
	}})
}

func (s *clientSuite) TestEnableHANumControllersPlacement(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.applicationService.EXPECT().AddControllerIAASUnits(gomock.Any(), []string{"0"}, []applicationservice.AddIAASUnitArg{{
		AddUnitArg: applicationservice.AddUnitArg{
			Placement: &instance.Placement{
				Scope:     instance.MachineScope,
				Directive: "4",
			},
		},
	}, {
		AddUnitArg: applicationservice.AddUnitArg{
			Placement: instance.MustParsePlacement("model-uuid:zone=us-east-1a"),
		},
	}, {
		AddUnitArg: applicationservice.AddUnitArg{},
	}}).Return([]coremachine.Name{"4", "99", "5"}, nil)

	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"0"}, nil)
	s.controllerNodeService.EXPECT().CurateNodes(gomock.Any(), []string{"4", "99", "5"}, nil).Return(nil)

	api := HighAvailabilityAPI{
		authorizer:            s.authorizer,
		blockCommandService:   s.blockCommandService,
		applicationService:    s.applicationService,
		controllerNodeService: s.controllerNodeService,
		isControllerModel:     true,
	}
	results, err := api.EnableHA(c.Context(), params.ControllersSpecs{
		Specs: []params.ControllersSpec{{
			NumControllers: 0,
			Placement:      []string{"4", "model-uuid:zone=us-east-1a"},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.DeepEquals, []params.ControllersChangeResult{{
		Result: params.ControllersChanges{
			Added:      []string{"4", "99", "5"},
			Maintained: []string{"0"},
		},
	}})
}

func (s *clientSuite) TestControllerDetailsPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.LoginAccess, gomock.Any()).Return(errors.New("boom"))

	api := HighAvailabilityAPI{
		controllerNodeService: s.controllerNodeService,
		authorizer:            s.authorizer,
	}
	_, err := api.ControllerDetails(c.Context())
	c.Assert(err, tc.DeepEquals, &params.Error{Message: "permission denied", Code: "unauthorized access"})
}

func (s *clientSuite) TestControllerDetailsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.LoginAccess, gomock.Any()).Return(nil)
	s.controllerNodeService.EXPECT().GetControllerAPIAddresses(gomock.Any()).Return(map[string]network.HostPorts{}, controllernodeerrors.EmptyAPIAddresses)

	api := HighAvailabilityAPI{
		controllerNodeService: s.controllerNodeService,
		authorizer:            s.authorizer,
	}
	results, err := api.ControllerDetails(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *clientSuite) TestControllerDetails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.LoginAccess, gomock.Any()).Return(nil)
	s.controllerNodeService.EXPECT().GetControllerAPIAddresses(gomock.Any()).Return(map[string]network.HostPorts{
		"0": network.MachineHostPorts{
			{
				MachineAddress: network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
				NetPort:        17070,
			},
		}.HostPorts(),
		"1": network.MachineHostPorts{
			{
				MachineAddress: network.NewMachineAddress("10.0.0.43", network.WithScope(network.ScopePublic)),
				NetPort:        17070,
			},
			{
				MachineAddress: network.NewMachineAddress("10.0.0.7", network.WithScope(network.ScopeCloudLocal)),
				NetPort:        17070,
			},
		}.HostPorts(),
	}, nil)

	api := HighAvailabilityAPI{
		controllerNodeService: s.controllerNodeService,
		authorizer:            s.authorizer,
	}
	results, err := api.ControllerDetails(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)

	c.Check(results.Results, tc.DeepEquals, []params.ControllerDetails{{
		ControllerId: "0",
		APIAddresses: []string{"10.0.0.1:17070"},
	}, {
		ControllerId: "1",
		APIAddresses: []string{"10.0.0.43:17070", "10.0.0.7:17070"},
	}})
}

func (s *clientSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)
	s.blockCommandService = NewMockBlockCommandService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)

	c.Cleanup(func() {
		s.authorizer = nil
		s.controllerNodeService = nil
		s.blockCommandService = nil
	})

	return ctrl
}
