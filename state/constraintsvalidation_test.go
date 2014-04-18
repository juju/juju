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
	consFallback string
	cons         string
	expected     string
}{
	{
		cons:         "root-disk=8G mem=4G arch=amd64",
		consFallback: "cpu-power=1000 cpu-cores=4",
		expected:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	}, {
		cons:         "root-disk=8G mem=4G arch=amd64",
		consFallback: "cpu-power=1000 cpu-cores=4 mem=8G",
		expected:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	}, {
		consFallback: "root-disk=8G cpu-cores=4 instance-type=foo",
		expected:     "root-disk=8G cpu-cores=4 instance-type=foo",
	}, {
		cons:     "root-disk=8G cpu-cores=4 instance-type=foo",
		expected: "root-disk=8G cpu-cores=4 instance-type=foo",
	}, {
		consFallback: "root-disk=8G instance-type=foo",
		cons:         "root-disk=8G cpu-cores=4 instance-type=bar",
		expected:     "root-disk=8G cpu-cores=4 instance-type=bar",
	}, {
		consFallback: "root-disk=8G mem=4G",
		cons:         "root-disk=8G cpu-cores=4 instance-type=bar",
		expected:     "root-disk=8G cpu-cores=4 instance-type=bar",
	}, {
		consFallback: "root-disk=8G cpu-cores=4 instance-type=bar",
		cons:         "root-disk=8G mem=4G",
		expected:     "root-disk=8G cpu-cores=4 mem=4G",
	}, {
		consFallback: "root-disk=8G cpu-cores=4 instance-type=bar",
		cons:         "root-disk=8G arch=amd64 mem=4G",
		expected:     "root-disk=8G cpu-cores=4 arch=amd64 mem=4G",
	},
}

func (s *constraintsValidationSuite) TestMachineConstraints(c *gc.C) {
	for i, t := range setConstraintsTests {
		c.Logf("test %d", i)
		err := s.State.SetEnvironConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, gc.IsNil)
		m, err := s.addOneMachine(c, constraints.MustParse(t.cons))
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
		err := s.State.SetEnvironConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, gc.IsNil)
		err = service.SetConstraints(constraints.MustParse(t.cons))
		c.Check(err, gc.IsNil)
		u, err := service.AddUnit()
		c.Check(err, gc.IsNil)
		cons, err := state.UnitConstraints(u)
		c.Check(err, gc.IsNil)
		c.Check(*cons, gc.DeepEquals, constraints.MustParse(t.expected))
	}
}
