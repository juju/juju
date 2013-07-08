// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type BenchmarkSuite struct {
}

var _ = Suite(&BenchmarkSuite{})

func (*BenchmarkSuite) BenchmarkAddUnit(c *C) {
	// TODO(rog) embed ConnSuite in BenchmarkSuite when
	// gocheck calls appropriate fixture methods for benchmark
	// functions.
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	charm := s.AddTestingCharm(c, "wordpress")
	svc, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		_, err := svc.AddUnit()
		c.Assert(err, IsNil)
	}
}

func (*BenchmarkSuite) BenchmarkAddAndAssignUnit(c *C) {
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	charm := s.AddTestingCharm(c, "wordpress")
	svc, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		unit, err := svc.AddUnit()
		c.Assert(err, IsNil)
		err = s.State.AssignUnit(unit, state.AssignClean)
		c.Assert(err, IsNil)
	}
}
