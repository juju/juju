package testing_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

type cleanupSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&cleanupSuite{})

func (s *cleanupSuite) TestTearDownSuiteEmpty(c *gc.C) {
	// The suite stack is empty initially, check we can tear that down.
	s.TearDownSuite(c)
	s.SetUpSuite(c)
}

func (s *cleanupSuite) TestTearDownTestEmpty(c *gc.C) {
	// The test stack is empty initially, check we can tear that down.
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func (s *cleanupSuite) TestAddSuiteCleanup(c *gc.C) {
	order := []string{}
	s.AddSuiteCleanup(func(*gc.C) {
		order = append(order, "first")
	})
	s.AddSuiteCleanup(func(*gc.C) {
		order = append(order, "second")
	})

	s.TearDownSuite(c)
	c.Assert(order, gc.DeepEquals, []string{"second", "first"})

	// and to avoid calling them twice, clear out the stack.
	s.SetUpSuite(c)
}

func (s *cleanupSuite) TestAddCleanup(c *gc.C) {
	order := []string{}
	s.AddCleanup(func(*gc.C) {
		order = append(order, "first")
	})
	s.AddCleanup(func(*gc.C) {
		order = append(order, "second")
	})

	s.TearDownTest(c)
	c.Assert(order, gc.DeepEquals, []string{"second", "first"})

	// and to avoid calling them twice, clear out the stack.
	s.SetUpTest(c)
}
