// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/gomaasapi"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/network"
)

type maas2InstanceSuite struct {
	maas2Suite
}

var _ = gc.Suite(&maas2InstanceSuite{})

func (s *maas2InstanceSuite) TestString(c *gc.C) {
	machine := &fakeMachine{hostname: "peewee", systemID: "herman"}
	instance := &maas2Instance{machine: machine}
	c.Assert(instance.String(), gc.Equals, "peewee:herman")
}

func (s *maas2InstanceSuite) TestID(c *gc.C) {
	machine := &fakeMachine{systemID: "herman"}
	thing := &maas2Instance{machine: machine}
	c.Assert(thing.Id(), gc.Equals, instance.Id("herman"))
}

func (s *maas2InstanceSuite) TestAddresses(c *gc.C) {
	vlan := fakeVLAN{vid: 66}
	subnet := fakeSubnet{
		id:    99,
		vlan:  vlan,
		cidr:  "192.168.10.0/24",
		space: "freckles",
	}
	machine := &fakeMachine{
		systemID: "1",
		interfaceSet: []gomaasapi.Interface{
			&fakeInterface{
				id:         91,
				name:       "eth0",
				type_:      "physical",
				enabled:    true,
				macAddress: "52:54:00:70:9b:fe",
				vlan:       vlan,
				links: []gomaasapi.Link{
					&fakeLink{
						id:        436,
						subnet:    &subnet,
						ipAddress: "192.168.10.1",
						mode:      "static",
					},
				},
				parents:  []string{},
				children: []string{},
			},
		},
	}
	controller := &fakeController{
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet},
			},
		},
		machines: []gomaasapi.Machine{machine},
	}
	instance := &maas2Instance{machine: machine, environ: s.makeEnviron(c, controller)}
	addresses, err := instance.Addresses(s.callCtx)

	expectedAddresses := []network.Address{
		newAddressOnSpaceWithId("freckles", corenetwork.Id("4567"), "192.168.10.1"),
	}

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.SameContents, expectedAddresses)
}

func (s *maas2InstanceSuite) TestZone(c *gc.C) {
	machine := &fakeMachine{zoneName: "inflatable"}
	instance := &maas2Instance{machine: machine}
	zone, err := instance.zone()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zone, gc.Equals, "inflatable")
}

func (s *maas2InstanceSuite) TestStatusSuccess(c *gc.C) {
	machine := &fakeMachine{statusMessage: "Wexler", statusName: "Deploying"}
	thing := &maas2Instance{machine: machine}
	result := thing.Status(s.callCtx)
	c.Assert(result, jc.DeepEquals, instance.Status{status.Allocating, "Deploying: Wexler"})
}

func (s *maas2InstanceSuite) TestStatusError(c *gc.C) {
	machine := &fakeMachine{statusMessage: "", statusName: ""}
	thing := &maas2Instance{machine: machine}
	result := thing.Status(s.callCtx)
	c.Assert(result, jc.DeepEquals, instance.Status{"", "error in getting status"})
}

func (s *maas2InstanceSuite) TestHostname(c *gc.C) {
	machine := &fakeMachine{hostname: "saul-goodman"}
	thing := &maas2Instance{machine: machine}
	result, err := thing.hostname()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "saul-goodman")
}

func (s *maas2InstanceSuite) TestHostnameIsDisplayName(c *gc.C) {
	machine := &fakeMachine{hostname: "saul-goodman"}
	thing := &maas2Instance{machine: machine}
	result, err := thing.displayName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "saul-goodman")
}

func (s *maas2InstanceSuite) TestDisplayNameFallsBackToFQDN(c *gc.C) {
	machine := newFakeMachine("abc123", "amd64", "ok")
	thing := &maas2Instance{machine: machine}
	result, err := thing.displayName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, thing.machine.FQDN())
}

func (s *maas2InstanceSuite) TestHardwareCharacteristics(c *gc.C) {
	machine := &fakeMachine{
		cpuCount:     3,
		memory:       4,
		architecture: "foo/bam",
		zoneName:     "bar",
		tags:         []string{"foo", "bar"},
	}
	thing := &maas2Instance{machine: machine}
	arch := "foo"
	cpu := uint64(3)
	mem := uint64(4)
	zone := "bar"
	tags := []string{"foo", "bar"}
	expected := &instance.HardwareCharacteristics{
		Arch:             &arch,
		CpuCores:         &cpu,
		Mem:              &mem,
		AvailabilityZone: &zone,
		Tags:             &tags,
	}
	result, err := thing.hardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expected)
}
