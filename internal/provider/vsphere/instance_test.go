// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/vim25/mo"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&InstanceSuite{})

func (s *InstanceSuite) TestInstances(c *gc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
		buildVM("inst-1").vm(),
		buildVM("inst-2").vm(),
	}
	instances, err := s.env.Instances(context.Background(), []instance.Id{"inst-0", "inst-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 2)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], gc.NotNil)
	c.Assert(instances[0].Id(), gc.Equals, instance.Id("inst-0"))
	c.Assert(instances[1].Id(), gc.Equals, instance.Id("inst-1"))
}

func (s *InstanceSuite) TestInstancesNoInstances(c *gc.C) {
	_, err := s.env.Instances(context.Background(), []instance.Id{"inst-0"})
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
}

func (s *InstanceSuite) TestInstancesPartialInstances(c *gc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
		buildVM("inst-1").vm(),
	}
	instances, err := s.env.Instances(context.Background(), []instance.Id{"inst-1", "inst-2"})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], gc.IsNil)
	c.Assert(instances[0].Id(), gc.Equals, instance.Id("inst-1"))
}

func (s *InstanceSuite) TestInstanceStatus(c *gc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
		buildVM("inst-1").powerOff().vm(),
	}
	instances, err := s.env.Instances(context.Background(), []instance.Id{"inst-0", "inst-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances[0].Status(context.Background()), jc.DeepEquals, instance.Status{
		Status:  status.Running,
		Message: "poweredOn",
	})
	c.Assert(instances[1].Status(context.Background()), jc.DeepEquals, instance.Status{
		Status:  status.Empty,
		Message: "poweredOff",
	})
}

func (s *InstanceSuite) TestInstanceAddresses(c *gc.C) {
	vm0 := buildVM("inst-0").nic(
		newNic("10.1.1.1", "10.1.1.2"),
		newNic("10.1.1.3"),
	).vm()
	vm1 := buildVM("inst-1").vm()
	vm2 := buildVM("inst-2").vm()
	vm2.Guest = nil

	s.client.virtualMachines = []*mo.VirtualMachine{vm0, vm1, vm2}
	instances, err := s.env.Instances(context.Background(), []instance.Id{"inst-0", "inst-1", "inst-2"})
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := instances[0].Addresses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, jc.DeepEquals, network.NewMachineAddresses([]string{"10.1.1.1", "10.1.1.2", "10.1.1.3"}).AsProviderAddresses())

	addrs, err = instances[1].Addresses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	addrs, err = instances[2].Addresses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)
}

func (s *InstanceSuite) TestControllerInstances(c *gc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
		buildVM("inst-1").extraConfig("juju-is-controller", "true").vm(),
	}
	ids, err := s.env.ControllerInstances(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, jc.DeepEquals, []instance.Id{"inst-1"})
}

func (s *InstanceSuite) TestOpenPortNoExternalNetwork(c *gc.C) {
	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").vm(),
	}
	envInstances, err := s.env.Instances(context.Background(), []instance.Id{"inst-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envInstances, gc.HasLen, 1)
	inst0 := envInstances[0]
	firewaller, ok := inst0.(instances.InstanceFirewaller)
	c.Assert(ok, jc.IsTrue)
	// machineID is ignored in per-instance firewallers
	err = firewaller.OpenPorts(context.Background(), "", firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("10/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = firewaller.ClosePorts(context.Background(), "", firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("10/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	c.Assert(err, jc.ErrorIsNil)
}
