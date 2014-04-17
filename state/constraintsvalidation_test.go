// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
)

type constraintsValidationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&constraintsValidationSuite{})

func (s *constraintsValidationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.getConstraintsValidator = func(*config.Config) (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem, constraints.Arch})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
}

func (s *constraintsValidationSuite) addOneMachine(c *gc.C, cons constraints.Value) (*state.Machine, error) {
	return s.State.AddOneMachine(state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
	})
}

var setConstraintsTests = []struct {
	consA    string
	consB    string
	expected string
}{
	{
		consB:    "root-disk=8G mem=4G arch=amd64",
		consA:    "cpu-power=1000 cpu-cores=4",
		expected: "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	}, {
		consB:    "root-disk=8G mem=4G arch=amd64",
		consA:    "cpu-power=1000 cpu-cores=4 mem=8G",
		expected: "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	}, {
		consA:    "root-disk=8G cpu-cores=4 instance-type=foo",
		expected: "root-disk=8G cpu-cores=4 instance-type=foo",
	}, {
		consB:    "root-disk=8G cpu-cores=4 instance-type=foo",
		expected: "root-disk=8G cpu-cores=4 instance-type=foo",
	}, {
		consA:    "root-disk=8G instance-type=foo",
		consB:    "root-disk=8G cpu-cores=4 instance-type=bar",
		expected: "root-disk=8G cpu-cores=4 instance-type=bar",
	}, {
		consA:    "root-disk=8G mem=4G",
		consB:    "root-disk=8G cpu-cores=4 instance-type=bar",
		expected: "root-disk=8G cpu-cores=4 instance-type=bar",
	}, {
		consA:    "root-disk=8G cpu-cores=4 instance-type=bar",
		consB:    "root-disk=8G mem=4G",
		expected: "root-disk=8G cpu-cores=4 mem=4G",
	}, {
		consA:    "root-disk=8G cpu-cores=4 instance-type=bar",
		consB:    "root-disk=8G arch=amd64 mem=4G",
		expected: "root-disk=8G cpu-cores=4 arch=amd64 mem=4G",
	},
}

func (s *constraintsValidationSuite) TestMachineConstraints(c *gc.C) {
	for i, t := range setConstraintsTests {
		c.Logf("test %d", i)
		err := s.State.SetEnvironConstraints(constraints.MustParse(t.consA))
		c.Check(err, gc.IsNil)
		m, err := s.addOneMachine(c, constraints.MustParse(t.consB))
		c.Check(err, gc.IsNil)
		cons, err := m.Constraints()
		c.Check(err, gc.IsNil)
		c.Check(cons, gc.DeepEquals, constraints.MustParse(t.expected))
	}
}

func (s *constraintsValidationSuite) TestServiceConstraints(c *gc.C) {
	charm := s.AddTestingCharm(c, "wordpress")
	service := s.AddTestingService(c, "wordpress", charm)
	for i, t := range setConstraintsTests {
		c.Logf("test %d", i)
		err := s.State.SetEnvironConstraints(constraints.MustParse(t.consA))
		c.Check(err, gc.IsNil)
		err = service.SetConstraints(constraints.MustParse(t.consB))
		c.Check(err, gc.IsNil)
		u, err := service.AddUnit()
		c.Check(err, gc.IsNil)
		cons, err := state.UnitConstraints(u)
		c.Check(err, gc.IsNil)
		c.Check(*cons, gc.DeepEquals, constraints.MustParse(t.expected))
	}
}
