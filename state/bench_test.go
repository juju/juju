// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type BenchmarkSuite struct {
}

var _ = gc.Suite(&BenchmarkSuite{})

func (*BenchmarkSuite) BenchmarkAddUnit(c *gc.C) {
	// TODO(rog) embed ConnSuite in BenchmarkSuite when
	// gocheck calls appropriate fixture methods for benchmark
	// functions.
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	charm := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", charm)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		_, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (*BenchmarkSuite) BenchmarkAddAndAssignUnit(c *gc.C) {
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	charm := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", charm)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		unit, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(unit, state.AssignClean)
		c.Assert(err, jc.ErrorIsNil)
	}
}
