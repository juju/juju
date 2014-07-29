// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environmentserver_test

import (
	"fmt"

	"github.com/juju/errors"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environmentserver"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type EnvironCapabilitySuite struct {
	testing.JujuConnSuite
	capability mockEnvironCapability
}

var _ = gc.Suite(&EnvironCapabilitySuite{})

type mockEnvironCapability struct {
	supportsUnitPlacementError error
}

func (p *mockEnvironCapability) SupportedArchitectures() ([]string, error) {
	panic("unused")
}

func (p *mockEnvironCapability) SupportNetworks() bool {
	panic("unused")
}

func (p *mockEnvironCapability) SupportsUnitPlacement() error {
	return p.supportsUnitPlacementError
}

func (s *EnvironCapabilitySuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.capability = mockEnvironCapability{}
	s.Deployer.GetEnvironCapability = func() (environmentserver.EnvironCapability, error) {
		return &s.capability, nil
	}
}

func (s *EnvironCapabilitySuite) addOneMachine(c *gc.C) (*state.Machine, error) {
	return s.State.EnvironmentDeployer.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
}

func (s *EnvironCapabilitySuite) addOneMachineWithInstanceId(c *gc.C) (*state.Machine, error) {
	return s.State.EnvironmentDeployer.AddOneMachine(state.MachineTemplate{
		Series:     "quantal",
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "i-rate",
		Nonce:      "ya",
	})
}

func (s *EnvironCapabilitySuite) addMachineInsideNewMachine(c *gc.C) error {
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXC)
	return err
}

func (s *EnvironCapabilitySuite) TestSupportsUnitPlacementAddMachine(c *gc.C) {
	// Ensure that AddOneMachine fails when SupportsUnitPlacement returns an error.
	s.capability.supportsUnitPlacementError = fmt.Errorf("no add-machine for you")
	_, err := s.addOneMachine(c)
	c.Assert(err, gc.ErrorMatches, ".*no add-machine for you")
	err = s.addMachineInsideNewMachine(c)
	c.Assert(err, gc.ErrorMatches, ".*no add-machine for you")
	// If the MockDeployer's EnvironCapability method fails, that will be returned first.
	s.Deployer.GetEnvironCapability = func() (environmentserver.EnvironCapability, error) {
		return nil, fmt.Errorf("incapable of EnvironCapability")
	}
	_, err = s.addOneMachine(c)
	c.Assert(err, gc.ErrorMatches, ".*incapable of EnvironCapability")
}

func (s *EnvironCapabilitySuite) TestSupportsUnitPlacementAddMachineInstanceId(c *gc.C) {
	// Ensure that AddOneMachine with a non-empty InstanceId does not fail.
	s.capability.supportsUnitPlacementError = fmt.Errorf("no add-machine for you")
	_, err := s.addOneMachineWithInstanceId(c)
	c.Assert(err, gc.IsNil)
}

func (s *EnvironCapabilitySuite) TestSupportsUnitPlacementUnitAssignment(c *gc.C) {
	m, err := s.addOneMachine(c)
	c.Assert(err, gc.IsNil)

	charm := s.AddTestingCharm(c, "wordpress")
	service := s.AddTestingService(c, "wordpress", charm)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)

	s.capability.supportsUnitPlacementError = fmt.Errorf("no unit placement for you")
	err = unit.AssignToMachine(m)
	c.Assert(err, gc.ErrorMatches, ".*no unit placement for you")

	err = unit.AssignToNewMachine()
	c.Assert(err, gc.IsNil)
}

func (s *EnvironCapabilitySuite) TestEnvironCapabilityUnimplemented(c *gc.C) {
	var capabilityErr error
	s.Deployer.GetEnvironCapability = func() (environmentserver.EnvironCapability, error) {
		return nil, capabilityErr
	}
	_, err := s.addOneMachine(c)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: MockDeployer returned nil EnvironCapability without an error")
	capabilityErr = errors.NotImplementedf("EnvironCapability")
	_, err = s.addOneMachine(c)
	c.Assert(err, gc.IsNil)
}

func (s *EnvironCapabilitySuite) TestSupportsUnitPlacementNoPolicy(c *gc.C) {
	s.Deployer.GetEnvironCapability = func() (environmentserver.EnvironCapability, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}
	_, err := s.addOneMachine(c)
	c.Assert(err, gc.IsNil)
}
