// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
)

type clientSuite struct {
	authorizer            *MockAuthorizer
	controllerNodeService *MockControllerNodeService
}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) TestStub(c *tc.C) {
	c.Skipf(`This suite is missing tests for the following scenarios:

 - Test s3 object store
 - Test enable HA error for no cloud local
 - Test enable HA error for no addresses
 - Test enable HA no error which verifies that virtual IPv4 addresses doesn't prevent enabling HA
 - Test enable HA no error which verifies that virtual IPv6 addresses doesn't prevent enabling HA
 - Test enable HA machine constraints
 - Test enable HA empty machine constraints
 - Test enable HA controller config constraints
 - Test enable HA controller config with file backed object store (currently not supported)
 - Test enable HA placement
 - Test enable HA placement --to
 - Test that killing a controller machine (machine-2) preserves the number of machines after enable-has is called again (machine-3 is added) to maintain 3 controller machines
 - Test that killing a controller machine (machine-4) preserves the number of machines after enable-has is called again (machine-5 is added) to maintain 5 controller machines
 - Test validate input options for enable-ha
 - Test enable HA with CAAS (currently not supported)
 `)
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

	c.Cleanup(func() {
		s.authorizer = nil
		s.controllerNodeService = nil
	})

	return ctrl
}
