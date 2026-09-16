// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
)

type bootstrapAddressSuite struct{}

func TestBootstrapAddressSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapAddressSuite{})
}

func (s *bootstrapAddressSuite) TestControllerServiceClusterIPAddresses(c *tc.C) {
	addresses, err := controllerServiceClusterIPAddresses(network.ProviderAddresses{
		network.NewMachineAddress("203.0.113.10", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		network.NewMachineAddress("10.152.183.205", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, network.ProviderAddresses{
		network.NewMachineAddress("10.152.183.205", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	})
}

func (s *bootstrapAddressSuite) TestControllerServiceClusterIPAddressesMissing(c *tc.C) {
	_, err := controllerServiceClusterIPAddresses(network.ProviderAddresses{
		network.NewMachineAddress("203.0.113.10", network.WithScope(network.ScopePublic)).AsProviderAddress(),
	})

	c.Check(err, tc.ErrorMatches, `controller service ClusterIP address not found`)
}
