// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"testing"

	"github.com/juju/tc"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
)

type InstanceSuite struct {
	EnvironFixture
}

func TestInstanceSuite(t *testing.T) {
	tc.Run(t, &InstanceSuite{})
}

func (s *InstanceSuite) TestInstances(c *tc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
		buildVM("inst-1").vm(),
		buildVM("inst-2").vm(),
	}
	instances, err := s.env.Instances(c.Context(), []instance.Id{"inst-0", "inst-1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.HasLen, 2)
	c.Assert(instances[0], tc.NotNil)
	c.Assert(instances[1], tc.NotNil)
	c.Assert(instances[0].Id(), tc.Equals, instance.Id("inst-0"))
	c.Assert(instances[1].Id(), tc.Equals, instance.Id("inst-1"))
}

func (s *InstanceSuite) TestInstancesNoInstances(c *tc.C) {
	_, err := s.env.Instances(c.Context(), []instance.Id{"inst-0"})
	c.Assert(err, tc.Equals, environs.ErrNoInstances)
}

func (s *InstanceSuite) TestInstancesPartialInstances(c *tc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
		buildVM("inst-1").vm(),
	}
	instances, err := s.env.Instances(c.Context(), []instance.Id{"inst-1", "inst-2"})
	c.Assert(err, tc.Equals, environs.ErrPartialInstances)
	c.Assert(instances[0], tc.NotNil)
	c.Assert(instances[1], tc.IsNil)
	c.Assert(instances[0].Id(), tc.Equals, instance.Id("inst-1"))
}

func (s *InstanceSuite) TestInstanceStatus(c *tc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
		buildVM("inst-1").powerOff().vm(),
	}
	instances, err := s.env.Instances(c.Context(), []instance.Id{"inst-0", "inst-1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances[0].Status(c.Context()), tc.DeepEquals, instance.Status{
		Status:  status.Running,
		Message: "poweredOn",
	})
	c.Assert(instances[1].Status(c.Context()), tc.DeepEquals, instance.Status{
		Status:  status.Empty,
		Message: "poweredOff",
	})
}

func (s *InstanceSuite) TestInstanceAddresses(c *tc.C) {
	vm0 := buildVM("inst-0").nic(
		newNic("10.1.1.1", "10.1.1.2"),
		newNic("10.1.1.3"),
	).vm()
	vm1 := buildVM("inst-1").vm()
	vm2 := buildVM("inst-2").vm()
	vm2.Guest = nil

	s.client.virtualMachines = []*mo.VirtualMachine{vm0, vm1, vm2}
	instances, err := s.env.Instances(c.Context(), []instance.Id{"inst-0", "inst-1", "inst-2"})
	c.Assert(err, tc.ErrorIsNil)

	addrs, err := instances[0].Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.DeepEquals, network.NewMachineAddresses([]string{"10.1.1.1", "10.1.1.2", "10.1.1.3"}).AsProviderAddresses())

	addrs, err = instances[1].Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)

	addrs, err = instances[2].Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.HasLen, 0)
}

func (s *InstanceSuite) TestControllerInstances(c *tc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
		buildVM("inst-1").extraConfig("juju-is-controller", "true").vm(),
	}
	ids, err := s.env.ControllerInstances(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.DeepEquals, []instance.Id{"inst-1"})
}

func (s *InstanceSuite) TestOpenPortNoExternalNetwork(c *tc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
	}
	envInstances, err := s.env.Instances(c.Context(), []instance.Id{"inst-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(envInstances, tc.HasLen, 1)
	inst0 := envInstances[0]
	firewaller, ok := inst0.(instances.InstanceFirewaller)
	c.Assert(ok, tc.IsTrue)
	// machineID is ignored in per-instance firewallers
	err = firewaller.OpenPorts(c.Context(), "", firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("10/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	c.Assert(err, tc.ErrorIsNil)
	err = firewaller.ClosePorts(c.Context(), "", firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("10/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	c.Assert(err, tc.ErrorIsNil)
}
