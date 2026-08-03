// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/instances"
)

type bootstrapAddressSuite struct{}

func TestBootstrapAddressSuite(t *testing.T) {
	tc.Run(t, &bootstrapAddressSuite{})
}

func (*bootstrapAddressSuite) TestInstanceAddresses(c *tc.C) {
	addresses := network.NewMachineAddresses([]string{"10.0.0.1"}).AsProviderAddresses()
	finder, err := NewBootstrapAddressFinder(instanceBootstrapEnviron{
		stubInstanceLister: stubInstanceLister{
			instances: []instances.Instance{
				stubInstance{id: "bootstrap", addresses: addresses},
			},
		},
	}, "bootstrap")
	c.Assert(err, tc.ErrorIsNil)

	got, err := finder.BootstrapControllerAddresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, addresses)
}

func (*bootstrapAddressSuite) TestInstanceLookupError(c *tc.C) {
	boom := errors.New("boom")
	finder := instanceBootstrapAddressFinder{
		lister:     stubInstanceLister{err: boom},
		instanceID: "bootstrap",
	}

	_, err := finder.BootstrapControllerAddresses(c.Context())
	c.Check(err, tc.ErrorIs, boom)
}

func (*bootstrapAddressSuite) TestPartialInstanceResult(c *tc.C) {
	addresses := network.NewMachineAddresses([]string{"10.0.0.1"}).AsProviderAddresses()
	finder := instanceBootstrapAddressFinder{lister: stubInstanceLister{
		instances: []instances.Instance{stubInstance{id: "bootstrap", addresses: addresses}},
		err:       ErrPartialInstances,
	}, instanceID: "bootstrap"}

	got, err := finder.BootstrapControllerAddresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, addresses)
}

func (*bootstrapAddressSuite) TestMissingInstance(c *tc.C) {
	finder := instanceBootstrapAddressFinder{
		lister:     stubInstanceLister{},
		instanceID: "bootstrap",
	}

	_, err := finder.BootstrapControllerAddresses(c.Context())
	c.Check(err, tc.ErrorMatches, `bootstrap instance "bootstrap" not found`)
}

func (*bootstrapAddressSuite) TestNilInstance(c *tc.C) {
	finder := instanceBootstrapAddressFinder{lister: stubInstanceLister{
		instances: []instances.Instance{nil},
		err:       ErrPartialInstances,
	}, instanceID: "bootstrap"}

	_, err := finder.BootstrapControllerAddresses(c.Context())
	c.Check(err, tc.ErrorMatches, `bootstrap instance "bootstrap" not found`)
}

func (*bootstrapAddressSuite) TestWrongInstance(c *tc.C) {
	finder := instanceBootstrapAddressFinder{lister: stubInstanceLister{
		instances: []instances.Instance{stubInstance{id: "other"}},
	}, instanceID: "bootstrap"}

	_, err := finder.BootstrapControllerAddresses(c.Context())
	c.Check(err, tc.ErrorMatches, `bootstrap instance "bootstrap" \(provider returned "other"\) not found`)
}

func (*bootstrapAddressSuite) TestInstanceAddressesError(c *tc.C) {
	boom := errors.New("boom")
	finder := instanceBootstrapAddressFinder{lister: stubInstanceLister{
		instances: []instances.Instance{stubInstance{id: "bootstrap", err: boom}},
	}, instanceID: "bootstrap"}

	_, err := finder.BootstrapControllerAddresses(c.Context())
	c.Check(err, tc.ErrorIs, boom)
}

func (*bootstrapAddressSuite) TestUnsupportedEnviron(c *tc.C) {
	_, err := NewBootstrapAddressFinder(unsupportedBootstrapEnviron{}, "bootstrap")
	c.Check(err, tc.ErrorMatches, `bootstrap controller addresses for environ environs.unsupportedBootstrapEnviron not supported`)
}

type instanceBootstrapEnviron struct {
	BootstrapEnviron
	stubInstanceLister
}

type unsupportedBootstrapEnviron struct {
	BootstrapEnviron
}

type stubInstanceLister struct {
	instances []instances.Instance
	err       error
}

func (s stubInstanceLister) Instances(context.Context, []instance.Id) ([]instances.Instance, error) {
	return s.instances, s.err
}

type stubInstance struct {
	id        instance.Id
	addresses network.ProviderAddresses
	err       error
}

func (s stubInstance) Id() instance.Id {
	return s.id
}

func (stubInstance) Status(context.Context) instance.Status {
	return instance.Status{}
}

func (s stubInstance) Addresses(context.Context) (network.ProviderAddresses, error) {
	return s.addresses, s.err
}
