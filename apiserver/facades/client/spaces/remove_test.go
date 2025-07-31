// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

type removeSpaceAPISuite struct {
	APIBaseSuite
}

func TestRemoveSpaceAPISuite(t *testing.T) {
	tc.Run(t, &removeSpaceAPISuite{})
}

// getRemoveSpaceArgs creates a params.RemoveSpaceParams with the given space name, force, and dryRun flags
func (s *removeSpaceAPISuite) getRemoveSpaceArgs(name network.SpaceName, force, dryRun bool) params.RemoveSpaceParams {
	return params.RemoveSpaceParams{
		SpaceParams: []params.RemoveSpaceParam{s.getRemoveSpaceArg(name, force, dryRun)},
	}
}
func (s *removeSpaceAPISuite) getRemoveSpaceArg(name network.SpaceName, force, dryRun bool) params.RemoveSpaceParam {
	spaceTag := names.NewSpaceTag(name.String())
	return params.RemoveSpaceParam{
		Space:  params.Entity{Tag: spaceTag.String()},
		Force:  force,
		DryRun: dryRun,
	}
}

// TestRemoveSpaceNotMutable tests that RemoveSpace returns an error when spaces
// are not mutable
func (s *removeSpaceAPISuite) TestRemoveSpaceNotMutable(c *tc.C) {
	// Set providerSpaces to true to make ensureSpacesNotProviderSourced fail
	defer s.SetupMocks(c, true, true).Finish()

	// Call RemoveSpace
	result, err := s.API.RemoveSpace(context.Background(), s.getRemoveSpaceArgs("test", false, false))

	// Verify that the error from ensureSpacesAreMutable is returned
	c.Assert(err, tc.ErrorMatches, ".*modifying provider-sourced spaces.*")
	// No Results are initialized when ensureSpacesAreMutable fails
	c.Assert(result.Results, tc.HasLen, 0)
}

// TestRemoveSpaceControllerConfigFails tests that RemoveSpace returns an error
// when controller config fetch fails
func (s *removeSpaceAPISuite) TestRemoveSpaceControllerConfigFails(c *tc.C) {
	defer s.SetupMocks(c, true, false).Finish()

	// Mock the controller config service to return an error
	configErr := errors.New("controller config error")
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{}, configErr)

	// Call RemoveSpace
	_, err := s.API.RemoveSpace(context.Background(), s.getRemoveSpaceArgs("test", false, false))

	// Verify that the error from controller config is returned
	c.Assert(err, tc.ErrorIs, configErr)
	c.Assert(err, tc.ErrorMatches, "retrieving controller config:.*")
}

// TestRemoveSpaceFails tests that RemoveSpace returns an error when the network service fails
func (s *removeSpaceAPISuite) TestRemoveSpaceFails(c *tc.C) {
	defer s.SetupMocks(c, true, false).Finish()

	// Mock the controller config service
	cfg := controller.Config{
		controller.JujuManagementSpace: "mgmt-space",
	}
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	// Mock the network service to return an error when removing the space
	removeErr := errors.New("remove space error")
	s.NetworkService.EXPECT().RemoveSpace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(domainnetwork.RemoveSpaceViolations{}, removeErr)

	// Call RemoveSpace
	_, err := s.API.RemoveSpace(context.Background(), s.getRemoveSpaceArgs("test", false, false))

	// Verify that the error from RemoveSpace is returned
	c.Assert(err, tc.ErrorIs, removeErr)
	c.Assert(err, tc.ErrorMatches, `removing space "test":.*`)
}

// TestRemoveSeveralSpaces tests that RemoveSpace can remove multiple spaces
func (s *removeSpaceAPISuite) TestRemoveSeveralSpaces(c *tc.C) {
	defer s.SetupMocks(c, true, false).Finish()

	// Mock the controller config service
	cfg := controller.Config{
		controller.JujuManagementSpace: "mgmt-space",
	}
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	// Mock the network service for the first space
	s.NetworkService.EXPECT().RemoveSpace(
		gomock.Any(),
		network.SpaceName("test1"),
		false,
		false,
	).Return(domainnetwork.RemoveSpaceViolations{}, nil)

	// Mock the network service for the second space
	s.NetworkService.EXPECT().RemoveSpace(
		gomock.Any(),
		network.SpaceName("test2"),
		true,
		false,
	).Return(domainnetwork.RemoveSpaceViolations{}, nil)

	// Call RemoveSpace with multiple spaces
	args := params.RemoveSpaceParams{
		SpaceParams: []params.RemoveSpaceParam{
			s.getRemoveSpaceArg("test1", false, false),
			s.getRemoveSpaceArg("test2", true, false),
		},
	}
	result, err := s.API.RemoveSpace(context.Background(), args)

	// Verify the result
	c.Assert(err, tc.IsNil)
	c.Check(result.Results, tc.DeepEquals, []params.RemoveSpaceResult{
		{}, {}, // two empty results, since no errors
	})
}

// TestRemoveJujuManagementSpace tests that RemoveSpace handles the
// JujuManagementSpace correctly
func (s *removeSpaceAPISuite) TestRemoveJujuManagementSpace(c *tc.C) {
	defer s.SetupMocks(c, true, false).Finish()

	// Mock the controller config service with a management space
	mgmtSpace := "mgmt-space"
	cfg := controller.Config{
		controller.JujuManagementSpace: mgmtSpace,
	}
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	// Mock the network service - note that dryRun is true when removing management space
	expectedViolations := domainnetwork.RemoveSpaceViolations{
		ApplicationConstraints: []string{"app-1"},
	}
	s.NetworkService.EXPECT().RemoveSpace(
		gomock.Any(),
		network.SpaceName(mgmtSpace),
		false,
		true, // dryRun should be true for management space
	).Return(expectedViolations, nil)

	// Call RemoveSpace for the management space
	result, err := s.API.RemoveSpace(context.Background(), s.getRemoveSpaceArgs(network.SpaceName(mgmtSpace), false, false))

	// Verify the result
	c.Assert(err, tc.IsNil)
	c.Check(result.Results, tc.DeepEquals, []params.RemoveSpaceResult{
		{
			Constraints: []params.Entity{
				{Tag: names.NewApplicationTag("app-1").String()},
			},
			ControllerSettings: []string{controller.JujuManagementSpace},
		},
	})
}

// TestRemoveSpaceWithViolations tests that RemoveSpace correctly handles violations
func (s *removeSpaceAPISuite) TestRemoveSpaceWithViolations(c *tc.C) {
	defer s.SetupMocks(c, true, false).Finish()

	// Mock the controller config service
	cfg := controller.Config{
		controller.JujuManagementSpace: "mgmt-space",
	}
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	// Create violations
	violations := domainnetwork.RemoveSpaceViolations{
		HasModelConstraint:     true,
		ApplicationConstraints: []string{"app1", "app2"},
		ApplicationBindings:    []string{"app3"},
	}

	// Mock the network service to return violations
	s.NetworkService.EXPECT().RemoveSpace(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(violations, nil)

	// Call RemoveSpace
	result, err := s.API.RemoveSpace(context.Background(), s.getRemoveSpaceArgs("test", false, false))

	// Verify the result
	c.Assert(err, tc.IsNil)
	c.Assert(result.Results, tc.HasLen, 1)

	// Check that bindings are correctly transformed
	c.Check(result.Results[0].Bindings, tc.SameContents,
		[]params.Entity{{Tag: names.NewApplicationTag("app3").String()}})

	// Check that constraints are correctly transformed
	c.Check(result.Results[0].Constraints, tc.SameContents, // 2 apps + model
		[]params.Entity{
			{Tag: names.NewApplicationTag("app1").String()},
			{Tag: names.NewApplicationTag("app2").String()},
			{Tag: s.API.modelTag.String()},
		})
}

// TestRemoveSpaceWithoutViolations tests that RemoveSpace correctly handles
// the case with no violations
func (s *removeSpaceAPISuite) TestRemoveSpaceWithoutViolations(c *tc.C) {
	defer s.SetupMocks(c, true, false).Finish()

	// Mock the controller config service
	cfg := controller.Config{
		controller.JujuManagementSpace: "mgmt-space",
	}
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	// Create empty violations
	violations := domainnetwork.RemoveSpaceViolations{}

	// Mock the network service to return no violations
	s.NetworkService.EXPECT().RemoveSpace(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(violations, nil)

	// Call RemoveSpace
	result, err := s.API.RemoveSpace(context.Background(), s.getRemoveSpaceArgs("test", false, false))

	// Verify the result
	c.Assert(err, tc.IsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Bindings, tc.HasLen, 0)
	c.Check(result.Results[0].Constraints, tc.HasLen, 0)
	c.Check(result.Results[0].ControllerSettings, tc.HasLen, 0)
}

// TestRemoveSpaceWithForce tests that RemoveSpace correctly handles the force flag
func (s *removeSpaceAPISuite) TestRemoveSpaceWithForce(c *tc.C) {
	defer s.SetupMocks(c, true, false).Finish()

	// Mock the controller config service
	cfg := controller.Config{
		controller.JujuManagementSpace: "mgmt-space",
	}
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	// Create violations that should be ignored due to force
	violations := domainnetwork.RemoveSpaceViolations{
		HasModelConstraint:     true,
		ApplicationConstraints: []string{"app1"},
		ApplicationBindings:    []string{"app2"},
	}

	// Mock the network service to return violations but with force=true
	s.NetworkService.EXPECT().RemoveSpace(
		gomock.Any(),
		network.SpaceName("test"),
		true, // force=true
		false,
	).Return(violations, nil)

	// Call RemoveSpace with force=true
	result, err := s.API.RemoveSpace(context.Background(), s.getRemoveSpaceArgs("test", true, false))

	// Verify the result - violation are ignored with force
	c.Assert(err, tc.IsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0], tc.DeepEquals, params.RemoveSpaceResult{})
}

// TestRemoveSpaceWithDryRun tests that RemoveSpace correctly handles the dryRun flag
func (s *removeSpaceAPISuite) TestRemoveSpaceWithDryRun(c *tc.C) {
	defer s.SetupMocks(c, true, false).Finish()

	// Mock the controller config service
	cfg := controller.Config{
		controller.JujuManagementSpace: "mgmt-space",
	}
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	// Create violations
	violations := domainnetwork.RemoveSpaceViolations{
		HasModelConstraint:     true,
		ApplicationConstraints: []string{"app1"},
		ApplicationBindings:    []string{"app2"},
	}

	// Mock the network service with dryRun=true
	s.NetworkService.EXPECT().RemoveSpace(
		gomock.Any(),
		network.SpaceName("test"),
		false,
		true, // dryRun=true
	).Return(violations, nil)

	// Call RemoveSpace with dryRun=true
	result, err := s.API.RemoveSpace(context.Background(), s.getRemoveSpaceArgs("test", false, true))

	// Verify the result
	c.Assert(err, tc.IsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Bindings, tc.HasLen, 1)
	c.Check(result.Results[0].Constraints, tc.HasLen, 2) // 1 app + model
}

// TestRemoveSpaceInvalidTag tests that RemoveSpace correctly handles invalid tags
func (s *removeSpaceAPISuite) TestRemoveSpaceInvalidTag(c *tc.C) {
	defer s.SetupMocks(c, true, false).Finish()

	// Mock the controller config service
	cfg := controller.Config{
		controller.JujuManagementSpace: "mgmt-space",
	}
	s.ControllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	// Call RemoveSpace with an invalid tag
	args := params.RemoveSpaceParams{
		SpaceParams: []params.RemoveSpaceParam{
			{
				Space:  params.Entity{Tag: "invalid-tag"},
				Force:  false,
				DryRun: false,
			},
			s.getRemoveSpaceArg("test", false, false),
		},
	}

	// Mock the network service with dryRun=true
	s.NetworkService.EXPECT().RemoveSpace(
		gomock.Any(),
		network.SpaceName("test"),
		false,
		false,
	).Return(domainnetwork.RemoveSpaceViolations{}, nil)
	result, err := s.API.RemoveSpace(context.Background(), args)

	// Verify the result
	c.Assert(err, tc.IsNil)
	c.Assert(result.Results, tc.HasLen, 2)
	c.Check(result.Results[0].Error, tc.ErrorMatches, `"invalid-tag" is not a valid tag`)
	c.Check(result.Results[1].Error, tc.IsNil)
}
