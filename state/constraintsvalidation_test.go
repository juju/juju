// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type constraintsValidationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&constraintsValidationSuite{})

func (s *constraintsValidationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func(*config.Config) (constraints.Validator, error) {
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
}{{
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
}, {
	consFallback: "cpu-cores=4 mem=4G networks=net1,^net3",
	cons:         "networks=net2,^net4 mem=4G",
	expected:     "cpu-cores=4 mem=4G networks=net2,^net4",
}, {
	consFallback: "cpu-cores=4 mem=2G networks=",
	cons:         "mem=4G networks=net1,^net3",
	expected:     "cpu-cores=4 mem=4G networks=net1,^net3",
}}

func (s *constraintsValidationSuite) TestMachineConstraints(c *gc.C) {
	for i, t := range setConstraintsTests {
		c.Logf("test %d: fallback: %q, cons: %q", i, t.consFallback, t.cons)
		err := s.State.SetEnvironConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, gc.IsNil)
		m, err := s.addOneMachine(c, constraints.MustParse(t.cons))
		c.Check(err, gc.IsNil)
		cons, err := m.Constraints()
		c.Check(err, gc.IsNil)
		c.Check(cons, jc.DeepEquals, constraints.MustParse(t.expected))
	}
}

func (s *constraintsValidationSuite) TestServiceConstraints(c *gc.C) {
	charm := s.AddTestingCharm(c, "wordpress")
	service := s.AddTestingService(c, "wordpress", charm)
	for i, t := range setConstraintsTests {
		c.Logf("test %d: fallback: %q, cons: %q", i, t.consFallback, t.cons)
		err := s.State.SetEnvironConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, gc.IsNil)
		err = service.SetConstraints(constraints.MustParse(t.cons))
		c.Check(err, gc.IsNil)
		u, err := service.AddUnit()
		c.Check(err, gc.IsNil)
		ucons, err := u.Constraints()
		c.Check(err, gc.IsNil)
		c.Check(*ucons, jc.DeepEquals, constraints.MustParse(t.expected))
	}
}
