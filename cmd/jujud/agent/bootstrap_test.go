// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

type BootstrapSuite struct {
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &BootstrapSuite{})
}

func (s *BootstrapSuite) TestBootstrapControllerAddressesUsesProviderInterface(c *tc.C) {
	want := network.NewMachineAddresses([]string{"controller-0.controller.svc.cluster.local"}).AsProviderAddresses()
	env := &stubBootstrapAddressEnviron{addresses: want}

	got, err := bootstrapControllerAddresses(c.Context(), env, "bootstrap-instance")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, want)
	c.Check(env.calls, tc.Equals, 1)
}

type stubBootstrapAddressEnviron struct {
	environs.BootstrapEnviron
	addresses network.ProviderAddresses
	calls     int
}

func (e *stubBootstrapAddressEnviron) BootstrapControllerAddresses(
	_ context.Context,
) (network.ProviderAddresses, error) {
	e.calls++
	return e.addresses, nil
}
