// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

type PlacementValidatorSuite struct {
	ConnSuite
	validator mockPlacementValidator
}

var _ = gc.Suite(&PlacementValidatorSuite{})

type mockPlacementValidator struct {
	validatePlacementError     error
	validatePlacementPlacement *instance.Placement
}

func (p *mockPlacementValidator) ValidatePlacement(placement *instance.Placement) error {
	p.validatePlacementPlacement = placement
	return p.validatePlacementError
}

func (s *PlacementValidatorSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.validator = mockPlacementValidator{}
	s.policy.getPlacementValidator = func(*config.Config) (state.PlacementValidator, error) {
		return &s.validator, nil
	}
}

func (s *PlacementValidatorSuite) addMachine(c *gc.C, placement *instance.Placement) error {
	template := state.MachineTemplate{
		Series: "precise",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineWithPlacement(template, placement)
	return err
}

func (s *PlacementValidatorSuite) TestValidatePlacement(c *gc.C) {
	placement := instance.MustParsePlacement("x:y")
	err := s.addMachine(c, placement)
	c.Assert(err, gc.IsNil)
	c.Assert(s.validator.validatePlacementPlacement, gc.Equals, placement)
}

func (s *PlacementValidatorSuite) TestValidatePlacementNoPlacement(c *gc.C) {
	// validator does not get invoked if Placement is nil
	s.validator.validatePlacementError = fmt.Errorf("no instance for you")
	err := s.addMachine(c, nil)
	c.Assert(err, gc.IsNil)
}

func (s *PlacementValidatorSuite) TestValidatePlacementError(c *gc.C) {
	placement := instance.MustParsePlacement("x:y")
	s.validator.validatePlacementError = fmt.Errorf("no instance for you")
	err := s.addMachine(c, placement)
	c.Assert(err, gc.ErrorMatches, ".*no instance for you")

	// If the policy's PlacementValidator method fails, that will be returned first.
	s.policy.getPlacementValidator = func(*config.Config) (state.PlacementValidator, error) {
		return nil, fmt.Errorf("no validator for you")
	}
	err = s.addMachine(c, placement)
	c.Assert(err, gc.ErrorMatches, ".*no validator for you")
}

func (s *PlacementValidatorSuite) TestPlacementValidatorUnimplemented(c *gc.C) {
	placement := instance.MustParsePlacement("x:y")
	var validatorErr error
	s.policy.getPlacementValidator = func(*config.Config) (state.PlacementValidator, error) {
		return nil, validatorErr
	}
	err := s.addMachine(c, placement)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: policy returned nil PlacementValidator without an error")
	validatorErr = errors.NewNotImplementedError("PlacementValidator")
	err = s.addMachine(c, placement)
	c.Assert(err, gc.IsNil)
}

func (s *PlacementValidatorSuite) TestNoPolicy(c *gc.C) {
	var called bool
	s.policy.getPlacementValidator = func(*config.Config) (state.PlacementValidator, error) {
		called = true
		return nil, nil
	}
	state.SetPolicy(s.State, nil)
	err := s.addMachine(c, instance.MustParsePlacement("x:y"))
	c.Assert(err, gc.IsNil)
	c.Assert(called, jc.IsFalse)
}

func (s *PlacementValidatorSuite) TestValidatePlacementInjectMachine(c *gc.C) {
	// PlacementValidator should not be acquired or used
	// when injecting a machine with an existing instance.
	var called bool
	s.policy.getPlacementValidator = func(*config.Config) (state.PlacementValidator, error) {
		called = true
		return nil, nil
	}
	template := state.MachineTemplate{
		InstanceId: instance.Id("bootstrap"),
		Series:     "precise",
		Nonce:      state.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobManageEnviron},
	}
	_, err := s.State.AddMachineWithPlacement(template, instance.MustParsePlacement("x:y"))
	c.Assert(err, gc.IsNil)
	c.Assert(called, jc.IsFalse)
}
