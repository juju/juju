// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type containerAZSuite struct {
	jujutesting.JujuConnSuite
}

func (s *containerAZSuite) TestContainerAvailabilityZone(c *gc.C) {
	availabilityZone := "ru-north-siberia"
	azMachine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Characteristics: &instance.HardwareCharacteristics{AvailabilityZone: &availabilityZone},
	})

	retAvailabilityZone, err := azMachine.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(availabilityZone, gc.Equals, retAvailabilityZone)

	// now add a container to that machine
	container := s.Factory.MakeMachineNested(c, azMachine.Id(), nil)
	c.Assert(err, jc.ErrorIsNil)

	containerAvailabilityZone, err := container.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(availabilityZone, gc.Equals, containerAvailabilityZone)
}

func (s *containerAZSuite) TestContainerNilAvailabilityZone(c *gc.C) {
	azMachine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Characteristics: &instance.HardwareCharacteristics{AvailabilityZone: nil},
	})

	retAvailabilityZone, err := azMachine.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert("", gc.Equals, retAvailabilityZone)

	// now add a container to that machine
	container := s.Factory.MakeMachineNested(c, azMachine.Id(), nil)
	c.Assert(err, jc.ErrorIsNil)

	containerAvailabilityZone, err := container.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert("", gc.Equals, containerAvailabilityZone)
}
