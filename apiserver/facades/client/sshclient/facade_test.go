// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade"
	sshclient "github.com/juju/juju/apiserver/facades/client/sshclient"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type facadeSuite struct {
	testhelpers.IsolationSuite
}

func TestFacadeSuite(t *stdtesting.T) {
	tc.Run(t, &facadeSuite{})
}

func (s *facadeSuite) TestStub(c *tc.C) {
	c.Skip(`Tests for this facade should be implemented when the facade is cut over to services`)
}

// TestAllAddressesPreservesDefaultRouteOrderAndDeduplicates checks that
// AllAddresses keeps the database's public and default-route ordering while
// removing duplicates, rather than sorting the addresses again by value.
func (s *facadeSuite) TestAllAddressesPreservesDefaultRouteOrderAndDeduplicates(c *tc.C) {
	// Arrange
	publicAddress := network.NewSpaceAddress("203.0.113.10", network.WithScope(network.ScopePublic))
	defaultRouteAddress := network.NewSpaceAddress("10.1.0.123", network.WithScope(network.ScopeCloudLocal))
	otherAddress := network.NewSpaceAddress("10.0.0.123", network.WithScope(network.ScopeCloudLocal))

	machineUUID := machine.UUID("machine-uuid")
	client, err := sshclient.InternalFacade(
		names.ControllerTag{},
		names.ModelTag{},
		nil,
		stubMachineService{uuid: machineUUID},
		stubNetworkService{addresses: network.SpaceAddresses{
			publicAddress,
			defaultRouteAddress,
			otherAddress,
			defaultRouteAddress,
		}},
		nil,
		nil,
		stubAuthorizer{},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	result, err := client.AllAddresses(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
	c.Check(result.Results[0].Addresses, tc.DeepEquals, []string{
		"203.0.113.10",
		"10.1.0.123",
		"10.0.0.123",
	})
}

type stubAuthorizer struct {
	facade.Authorizer
}

func (stubAuthorizer) AuthClient() bool { return true }

func (stubAuthorizer) HasPermission(
	context.Context, permission.Access, names.Tag,
) error {
	return nil
}

type stubMachineService struct {
	uuid machine.UUID
}

func (s stubMachineService) GetMachineUUID(
	_ context.Context, _ machine.Name,
) (machine.UUID, error) {
	return s.uuid, nil
}

func (stubMachineService) GetSSHHostKeys(
	context.Context, machine.UUID,
) ([]string, error) {
	return nil, nil
}

type stubNetworkService struct {
	addresses network.SpaceAddresses
}

func (s stubNetworkService) GetMachineAddresses(
	_ context.Context, _ machine.UUID,
) (network.SpaceAddresses, error) {
	return s.addresses, nil
}

func (stubNetworkService) GetMachinePublicAddress(
	_ context.Context, _ machine.UUID,
) (network.SpaceAddress, error) {
	return network.SpaceAddress{}, nil
}

func (stubNetworkService) GetMachinePrivateAddress(
	_ context.Context, _ machine.UUID,
) (network.SpaceAddress, error) {
	return network.SpaceAddress{}, nil
}
