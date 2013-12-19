// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
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
	svc := s.AddTestingService(c, "wordpress", charm)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		_, err := svc.AddUnit()
		c.Assert(err, gc.IsNil)
	}
}

func (*BenchmarkSuite) BenchmarkAddAndAssignUnit(c *gc.C) {
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	charm := s.AddTestingCharm(c, "wordpress")
	svc := s.AddTestingService(c, "wordpress", charm)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		unit, err := svc.AddUnit()
		c.Assert(err, gc.IsNil)
		err = s.State.AssignUnit(unit, state.AssignClean)
		c.Assert(err, gc.IsNil)
	}
}
