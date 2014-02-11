// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

type PrecheckerSuite struct {
	ConnSuite
}

var _ = gc.Suite(&PrecheckerSuite{})

type mockPrechecker struct {
	precheckInstanceError       error
	precheckInstanceSeries      string
	precheckInstanceConstraints constraints.Value

	precheckContainerError  error
	precheckContainerSeries string
	precheckContainerKind   instance.ContainerType
}

func (p *mockPrechecker) PrecheckInstance(series string, cons constraints.Value) error {
	p.precheckInstanceSeries = series
	p.precheckInstanceConstraints = cons
	return p.precheckInstanceError
}

func (p *mockPrechecker) PrecheckContainer(series string, kind instance.ContainerType) error {
	p.precheckContainerSeries = series
	p.precheckContainerKind = kind
	return p.precheckContainerError
}

func (s *PrecheckerSuite) TestSetPrechecker(c *gc.C) {
	p := &mockPrechecker{}
	prev := s.State.SetPrechecker(p)
	c.Assert(prev, gc.IsNil)
	prev = s.State.SetPrechecker(nil)
	c.Assert(prev, gc.Equals, p)
}

func (s *PrecheckerSuite) TestPrecheckInstance(c *gc.C) {
	p := &mockPrechecker{}
	s.State.SetPrechecker(p)

	// PrecheckInstance should be called with the specified
	// series, and the specified constraints merged with the
	// environment constraints, when attempting to create an
	// instance.
	envCons := constraints.MustParse("mem=4G")
	err := s.State.SetEnvironConstraints(envCons)
	c.Assert(err, gc.IsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cpu-cores=4")
	template := state.MachineTemplate{
		Series:      "precise",
		Constraints: extraCons,
		Jobs:        oneJob,
	}
	_, err = s.State.AddOneMachine(template)
	c.Assert(err, gc.IsNil)
	c.Assert(p.precheckInstanceSeries, gc.Equals, template.Series)
	cons := template.Constraints.WithFallbacks(envCons)
	c.Assert(p.precheckInstanceConstraints, gc.DeepEquals, cons)

	// Ensure that AddOneMachine fails when PrecheckInstance returns an error.
	p.precheckInstanceError = fmt.Errorf("no instance for you")
	_, err = s.State.AddOneMachine(template)
	c.Assert(err, gc.ErrorMatches, ".*no instance for you")
}

func (s *PrecheckerSuite) TestPrecheckInstanceInjectMachine(c *gc.C) {
	p := &mockPrechecker{}
	s.State.SetPrechecker(p)
	template := state.MachineTemplate{
		InstanceId: instance.Id("bootstrap"),
		Series:     "precise",
		Nonce:      state.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobManageEnviron},
	}
	_, err := s.State.AddOneMachine(template)
	c.Assert(err, gc.IsNil)
	// PrecheckInstance should not have been called, as we've
	// injected a machine with an existing instance.
	c.Assert(p.precheckInstanceSeries, gc.Equals, "")
}

func (s *PrecheckerSuite) TestPrecheckContainer(c *gc.C) {
	m0, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	p := &mockPrechecker{}
	s.State.SetPrechecker(p)

	template := state.MachineTemplate{
		Series: "precise",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err = s.State.AddMachineInsideMachine(template, m0.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)
	c.Assert(p.precheckInstanceSeries, gc.Equals, "") // no call
	c.Assert(p.precheckContainerSeries, gc.Equals, template.Series)
	c.Assert(p.precheckContainerKind, gc.Equals, instance.LXC)

	// Ensure that AddMachineInsideMachine fails when PrecheckContainer returns an error.
	p.precheckContainerError = fmt.Errorf("no container for you")
	_, err = s.State.AddMachineInsideMachine(template, m0.Id(), instance.LXC)
	c.Assert(err, gc.ErrorMatches, ".*no container for you")
}

func (s *PrecheckerSuite) TestPrecheckContainerNewMachine(c *gc.C) {
	// Attempting to add a container to a new machine should cause
	// both PrecheckInstance and PrecheckContainer to be called.
	p := &mockPrechecker{}
	s.State.SetPrechecker(p)
	template := state.MachineTemplate{
		Series: "precise",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXC)
	c.Assert(err, gc.IsNil)
	c.Assert(p.precheckInstanceSeries, gc.Equals, template.Series)
	c.Assert(p.precheckContainerSeries, gc.Equals, template.Series)
	c.Assert(p.precheckContainerKind, gc.Equals, instance.LXC)
}
