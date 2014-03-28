// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type EnsureHASuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&EnsureHASuite{})

func runEnsureHA(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, &EnsureHACommand{}, args)
	return err
}

func (s *EnsureHASuite) TestEnsureHA(c *gc.C) {
	err := runEnsureHA(c)
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, state.Alive)
	c.Assert(m.Series(), gc.DeepEquals, "precise")
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
}

func (s *EnsureHASuite) TestEnsureHAWithSeries(c *gc.C) {
	err := runEnsureHA(c, "--series", "series")
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Series(), gc.DeepEquals, "series")
}

func (s *EnsureHASuite) TestEnsureHAWithConstraints(c *gc.C) {
	err := runEnsureHA(c, "--constraints", "mem=4G")
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	expectedCons := constraints.MustParse("mem=4G")
	c.Assert(mcons, gc.DeepEquals, expectedCons)
}

func (s *EnsureHASuite) TestEnsureHAIdempotent(c *gc.C) {
	for i := 0; i < 2; i++ {
		err := runEnsureHA(c, "-n", "1")
		c.Assert(err, gc.IsNil)
	}
	m, err := s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)

	// Running ensure-ha with constraints or series
	// will have no effect unless new machines are
	// created.
	err = runEnsureHA(c, "-n", "1", "--constraints", "mem=4G")
	c.Assert(err, gc.IsNil)
	m, err = s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	mcons, err = m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
}

func (s *EnsureHASuite) TestEnsureHAMultiple(c *gc.C) {
	err := runEnsureHA(c, "-n", "1")
	c.Assert(err, gc.IsNil)
	err = runEnsureHA(c, "-n", "3", "--constraints", "mem=4G")
	c.Assert(err, gc.IsNil)

	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
	mcons, err := machines[0].Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	for i := 1; i < 3; i++ {
		mcons, err := machines[i].Constraints()
		c.Assert(err, gc.IsNil)
		expectedCons := constraints.MustParse("mem=4G")
		c.Assert(mcons, gc.DeepEquals, expectedCons)
	}
}

func (s *EnsureHASuite) TestEnsureHAErrors(c *gc.C) {
	for _, n := range []int{-1, 0, 2} {
		err := runEnsureHA(c, "-n", fmt.Sprint(n))
		c.Assert(err, gc.ErrorMatches, "number of state servers must be odd and greater than zero")
	}
	err := runEnsureHA(c, "-n", "3")
	c.Assert(err, gc.IsNil)
	err = runEnsureHA(c, "-n", "1")
	c.Assert(err, gc.ErrorMatches, "error ensuring availability: cannot reduce state server count")
}
