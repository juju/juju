// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
)

func (s *environSuite) TestSubnetsInstanceIDError(c *gc.C) {
	env := s.openEnviron(c)

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)

	_, err := netEnv.Subnets(s.callCtx, "some-ID", nil)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *environSuite) TestSubnetsSuccess(c *gc.C) {
	env := s.openEnviron(c)

	// We wait for common resource creation, then query subnets
	// in the default virtual network created for every model.
	s.sender = azuretesting.Senders{
		makeSender("/deployments/common", s.commonDeployment),
		makeSender("/virtualNetworks/juju-internal-network/subnets", network.SubnetListResult{
			Value: &[]network.Subnet{
				{
					ID: to.StringPtr("provider-sub-id"),
					SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
						AddressPrefix: to.StringPtr("10.0.0.0/24"),
					},
				},
				{
					// Result without an address prefix is ignored.
					SubnetPropertiesFormat: &network.SubnetPropertiesFormat{},
				},
			},
		}),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)

	subs, err := netEnv.Subnets(s.callCtx, instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subs, gc.HasLen, 1)
	c.Check(subs[0].ProviderId, gc.Equals, corenetwork.Id("provider-sub-id"))
	c.Check(subs[0].CIDR, gc.Equals, "10.0.0.0/24")
}
