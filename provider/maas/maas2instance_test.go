// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
)

type maas2InstanceSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&maas2InstanceSuite{})

func (s *maas2InstanceSuite) TestString(c *gc.C) {
	machine := &fakeMachine{hostname: "peewee", systemID: "herman"}
	instance := &maas2Instance{machine: machine,
		maasController: &fakeMachineOnlyController{[]gomaasapi.Machine{machine}}}
	c.Assert(instance.String(), gc.Equals, "peewee:herman")
}

func (s *maas2InstanceSuite) TestID(c *gc.C) {
	machine := &fakeMachine{systemID: "herman"}
	thing := &maas2Instance{machine: machine,
		maasController: &fakeMachineOnlyController{[]gomaasapi.Machine{machine}}}
	c.Assert(thing.Id(), gc.Equals, instance.Id("herman"))
}

func (s *maas2InstanceSuite) TestAddresses(c *gc.C) {
	machine := &fakeMachine{ipAddresses: []string{
		"0.0.0.0",
		"1.2.3.4",
		"127.0.0.1",
	}}
	instance := &maas2Instance{machine: machine,
		maasController: &fakeMachineOnlyController{[]gomaasapi.Machine{machine}}}
	expectedAddresses := []network.Address{
		network.NewAddress("0.0.0.0"),
		network.NewAddress("1.2.3.4"),
		network.NewAddress("127.0.0.1"),
	}
	addresses, err := instance.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.SameContents, expectedAddresses)
}

func (s *maas2InstanceSuite) TestZone(c *gc.C) {
	machine := &fakeMachine{zoneName: "inflatable"}
	instance := &maas2Instance{machine: machine,
		maasController: &fakeMachineOnlyController{[]gomaasapi.Machine{machine}}}
	zone, err := instance.zone()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zone, gc.Equals, "inflatable")
}

func (s *maas2InstanceSuite) TestStatusSuccess(c *gc.C) {
	machine := &fakeMachine{statusMessage: "Wexler", statusName: "Deploying"}
	thing := &maas2Instance{machine: machine,
		maasController: &fakeMachineOnlyController{[]gomaasapi.Machine{machine}}}
	result := thing.Status()
	c.Assert(result, jc.DeepEquals, instance.InstanceStatus{status.Allocating, "Deploying: Wexler"})
}

func (s *maas2InstanceSuite) TestStatusError(c *gc.C) {
	machine := &fakeMachine{statusMessage: "", statusName: ""}
	thing := &maas2Instance{machine: machine,
		maasController: &fakeMachineOnlyController{[]gomaasapi.Machine{machine}}}
	result := thing.Status()
	c.Assert(result, jc.DeepEquals, instance.InstanceStatus{"", "error in getting status"})
}

func (s *maas2InstanceSuite) TestHostname(c *gc.C) {
	machine := &fakeMachine{hostname: "saul-goodman"}
	thing := &maas2Instance{machine: machine,
		maasController: &fakeMachineOnlyController{[]gomaasapi.Machine{machine}}}
	result, err := thing.hostname()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "saul-goodman")
}

func (s *maas2InstanceSuite) TestHardwareCharacteristics(c *gc.C) {
	machine := &fakeMachine{
		cpuCount:     3,
		memory:       4,
		architecture: "foo/bam",
		zoneName:     "bar",
		tags:         []string{"foo", "bar"},
	}
	thing := &maas2Instance{machine: machine,
		maasController: &fakeMachineOnlyController{[]gomaasapi.Machine{machine}}}
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
