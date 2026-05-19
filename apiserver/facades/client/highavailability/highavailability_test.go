// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/canonical/gomock/gomock"

	"github.com/juju/juju/core/permission"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/rpc/params"
)

type clientSuite struct {
	authorizer            *MockAuthorizer
	controllerNodeService *MockControllerNodeService
}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
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
	s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForClients(gomock.Any()).Return(map[string][]string{}, controllernodeerrors.EmptyAPIAddresses)

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
	s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForClients(gomock.Any()).Return(map[string][]string{
		"0": {"10.0.0.1:17070"},
		"1": {"10.0.0.43:17070", "10.0.0.7:17070"},
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
