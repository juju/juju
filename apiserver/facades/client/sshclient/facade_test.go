// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/sshclient"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type facadeSuite struct {
	testhelpers.IsolationSuite
}

func TestFacadeSuite(t *stdtesting.T) {
	tc.Run(t, &facadeSuite{})
}

func (s *facadeSuite) newFacade(c *tc.C, modelType model.ModelType) *sshclient.Facade {
	adminTag := names.NewUserTag("admin")
	auth := apiservertesting.FakeAuthorizer{Tag: adminTag, AdminTag: adminTag}
	facade, err := sshclient.InternalFacade(
		coretesting.ControllerTag, coretesting.ModelTag, modelType,
		nil, nil, nil, nil, nil, nil, auth,
	)
	c.Assert(err, tc.ErrorIsNil)
	return facade
}

func (s *facadeSuite) TestVirtualHostnameRejectsMachinesForCAAS(c *tc.C) {
	facade := s.newFacade(c, model.CAAS)
	for _, machineID := range []string{"0", "0/lxd/1"} {
		result, err := facade.VirtualHostname(c.Context(), params.VirtualHostnameTargetArg{
			Tag: names.NewMachineTag(machineID).String(),
		})
		c.Check(err, tc.ErrorMatches,
			`cannot SSH to machine "[^"]+" in a Kubernetes model; specify a unit name, such as <app>/0`)
		c.Assert(result.Error, tc.NotNil)
		c.Check(result.Error.Message, tc.Equals, err.Error())
		c.Check(result.Address, tc.Equals, "")
	}
}

func (s *facadeSuite) TestVirtualHostnameMachineForIAAS(c *tc.C) {
	facade := s.newFacade(c, model.IAAS)
	result, err := facade.VirtualHostname(c.Context(), params.VirtualHostnameTargetArg{
		Tag: names.NewMachineTag("0").String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Error, tc.IsNil)
	info, err := virtualhostname.NewInfoMachineTarget(coretesting.ModelTag.Id(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Address, tc.Equals, info.String())
}

func (s *facadeSuite) TestVirtualHostnameUnitForCAAS(c *tc.C) {
	facade := s.newFacade(c, model.CAAS)
	container := "charm"
	result, err := facade.VirtualHostname(c.Context(), params.VirtualHostnameTargetArg{
		Tag: names.NewUnitTag("redis/0").String(), Container: &container,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Error, tc.IsNil)
	info, err := virtualhostname.NewInfoContainerTarget(coretesting.ModelTag.Id(), "redis/0", container)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Address, tc.Equals, info.String())
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
		model.IAAS,
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
