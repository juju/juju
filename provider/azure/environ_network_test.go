// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
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
		makeSender("/virtualNetworks/juju-internal-network/subnets", nil),
	}

	netEnv, ok := environs.SupportsNetworking(env)
	c.Assert(ok, jc.IsTrue)

	_, err := netEnv.Subnets(s.callCtx, instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)
}
