// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/presence"
	coretesting "github.com/juju/juju/testing"
)

type EnsureAvailabilitySuite struct {
	jujutesting.RepoSuite
	machine0Pinger *presence.Pinger
}

var _ = gc.Suite(&EnsureAvailabilitySuite{})

func (s *EnsureAvailabilitySuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	// Add a state server to the environment, and ensure that it is
	// considered 'alive' so that calls don't spawn new instances
	_, err := s.State.AddMachine("precise", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.BackingState.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	s.machine0Pinger, err = m.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
	err = m.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnsureAvailabilitySuite) TearDownTest(c *gc.C) {
	// We have to Kill the Pinger before TearDownTest, otherwise the State
	// connection is already closed.
	if s.machine0Pinger != nil {
		s.machine0Pinger.Kill()
		s.machine0Pinger = nil
	}
	s.RepoSuite.TearDownTest(c)
}

func runEnsureAvailability(c *gc.C, args ...string) (*cmd.Context, error) {
	return coretesting.RunCommand(c, envcmd.Wrap(&EnsureAvailabilityCommand{}), args...)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailability(c *gc.C) {
	ctx, err := runEnsureAvailability(c, "-n", "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")

	m, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Alive)
	c.Assert(m.Series(), gc.DeepEquals, "precise")
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
}

func (s *EnsureAvailabilitySuite) TestBlockEnsureAvailability(c *gc.C) {
	// Block operation
	s.AssertConfigParameterUpdated(c, "block-all-changes", true)

	_, err := runEnsureAvailability(c, "-n", "1")
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())

	m, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Alive)

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To unblock changes.*")
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityPlacementError(c *gc.C) {
	_, err := runEnsureAvailability(c, "-n", "1", "--to", "1")
	c.Assert(err, gc.ErrorMatches, `unsupported ensure-availability placement directive "1"`)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityFormatYaml(c *gc.C) {
	expected := map[string][]string{
		"maintained": []string{"0"},
		"added":      []string{"1", "2"},
	}

	ctx, err := runEnsureAvailability(c, "-n", "3", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)

	var result map[string][]string
	err = goyaml.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityFormatJson(c *gc.C) {
	expected := map[string][]string{
		"maintained": []string{"0"},
		"added":      []string{"1", "2"},
	}

	ctx, err := runEnsureAvailability(c, "-n", "3", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)

	var result map[string][]string
	err = json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityWithSeries(c *gc.C) {
	ctx, err := runEnsureAvailability(c, "--series", "series", "-n", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	m, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Series(), gc.DeepEquals, "series")
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityWithConstraints(c *gc.C) {
	ctx, err := runEnsureAvailability(c, "--constraints", "mem=4G", "-n", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	m, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	expectedCons := constraints.MustParse("mem=4G")
	c.Assert(mcons, gc.DeepEquals, expectedCons)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityWithPlacement(c *gc.C) {
	ctx, err := runEnsureAvailability(c, "--to", "valid", "-n", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	m, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Placement(), gc.Equals, "valid")
	m, err = s.State.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Placement(), gc.Equals, "")
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityIdempotent(c *gc.C) {
	for i := 0; i < 2; i++ {
		_, err := runEnsureAvailability(c, "-n", "1")
		c.Assert(err, jc.ErrorIsNil)
	}
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	m, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	mcons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)

	// Running ensure-availability with constraints or series
	// will have no effect unless new machines are
	// created.
	ctx, err := runEnsureAvailability(c, "-n", "1", "--constraints", "mem=4G")
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")

	c.Assert(err, jc.ErrorIsNil)
	m, err = s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	mcons, err = m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityMultiple(c *gc.C) {
	ctx, err := runEnsureAvailability(c, "-n", "3", "--constraints", "mem=4G")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	mcons, err := machines[0].Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	for i := 1; i < 3; i++ {
		mcons, err := machines[i].Constraints()
		c.Assert(err, jc.ErrorIsNil)
		expectedCons := constraints.MustParse("mem=4G")
		c.Assert(mcons, gc.DeepEquals, expectedCons)
	}
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityErrors(c *gc.C) {
	for _, n := range []int{-1, 2} {
		_, err := runEnsureAvailability(c, "-n", fmt.Sprint(n))
		c.Assert(err, gc.ErrorMatches, "must specify a number of state servers odd and non-negative")
	}
	ctx, err := runEnsureAvailability(c, "-n", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	_, err = runEnsureAvailability(c, "-n", "1")
	c.Assert(err, gc.ErrorMatches, "failed to create new state server machines: cannot reduce state server count")
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityAllows0(c *gc.C) {
	ctx, err := runEnsureAvailability(c, "-n", "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityDefaultsTo3(c *gc.C) {
	ctx, err := runEnsureAvailability(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
}
