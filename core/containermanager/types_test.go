// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containermanager

import (
	stderrors "errors"
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/modelconfig"
)

type resolveNetworkingMethodSuite struct{}

func TestResolveNetworkingMethodSuite(t *testing.T) {
	tc.Run(t, &resolveNetworkingMethodSuite{})
}

func (s *resolveNetworkingMethodSuite) TestExplicitLocal(c *tc.C) {
	// An explicitly configured local method never consults the provider
	// capability: the result is local either way.
	for _, supports := range []bool{true, false} {
		method, err := ResolveNetworkingMethod(
			modelconfig.ContainerNetworkingMethodLocal, supports,
		)
		c.Check(err, tc.ErrorIsNil)
		c.Check(method, tc.Equals, NetworkingMethodLocal)
	}
}

func (s *resolveNetworkingMethodSuite) TestExplicitProvider(c *tc.C) {
	for _, supports := range []bool{true, false} {
		method, err := ResolveNetworkingMethod(
			modelconfig.ContainerNetworkingMethodProvider, supports,
		)
		c.Check(err, tc.ErrorIsNil)
		c.Check(method, tc.Equals, NetworkingMethodProvider)
	}
}

func (s *resolveNetworkingMethodSuite) TestAutoResolvesByProviderCapability(c *tc.C) {
	// The unset "auto" value resolves using whether the model's provider
	// supports allocating container addresses.
	method, err := ResolveNetworkingMethod(
		modelconfig.ContainerNetworkingMethodAuto, true,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, NetworkingMethodProvider)

	method, err = ResolveNetworkingMethod(
		modelconfig.ContainerNetworkingMethodAuto, false,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, NetworkingMethodLocal)
}

func (s *resolveNetworkingMethodSuite) TestNotValid(c *tc.C) {
	_, err := ResolveNetworkingMethod("bogus", false)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *resolveNetworkingMethodSuite) TestWithCapabilityExplicitNeverConsultsProvider(c *tc.C) {
	// Explicitly configured methods resolve without consulting the
	// provider capability.
	capabilityCalls := 0
	caps := func() (bool, error) {
		capabilityCalls++
		return true, nil
	}

	method, err := ResolveNetworkingMethodWithCapability(
		modelconfig.ContainerNetworkingMethodLocal, caps,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, NetworkingMethodLocal)

	method, err = ResolveNetworkingMethodWithCapability(
		modelconfig.ContainerNetworkingMethodProvider, caps,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, NetworkingMethodProvider)

	c.Check(capabilityCalls, tc.Equals, 0)
}

func (s *resolveNetworkingMethodSuite) TestWithCapabilityAuto(c *tc.C) {
	// The unset "auto" value is resolved using the model provider's
	// container address capability.
	method, err := ResolveNetworkingMethodWithCapability(
		modelconfig.ContainerNetworkingMethodAuto,
		func() (bool, error) { return true, nil },
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, NetworkingMethodProvider)

	method, err = ResolveNetworkingMethodWithCapability(
		modelconfig.ContainerNetworkingMethodAuto,
		func() (bool, error) { return false, nil },
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, NetworkingMethodLocal)
}

func (s *resolveNetworkingMethodSuite) TestWithCapabilityError(c *tc.C) {
	// Errors from the capability determination propagate.
	capErr := stderrors.New("boom")
	_, err := ResolveNetworkingMethodWithCapability(
		modelconfig.ContainerNetworkingMethodAuto,
		func() (bool, error) { return false, capErr },
	)
	c.Check(err, tc.ErrorIs, capErr)
}

func (s *resolveNetworkingMethodSuite) TestWithCapabilityNotValid(c *tc.C) {
	// An unknown value is rejected without consulting the provider
	// capability.
	capabilityCalls := 0
	_, err := ResolveNetworkingMethodWithCapability("bogus", func() (bool, error) {
		capabilityCalls++
		return true, nil
	})
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(capabilityCalls, tc.Equals, 0)
}
