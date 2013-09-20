// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase_test

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type cleanupSuite struct {
	testbase.CleanupSuite
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

	// SetUpSuite resets the cleanup stack, this stops the cleanup functions
	// being called again.
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

	// SetUpTest resets the cleanup stack, this stops the cleanup functions
	// being called again.
	s.SetUpTest(c)
}

func (s *cleanupSuite) TestPatchEnvironment(c *gc.C) {
	const envName = "TESTING_PATCH_ENVIRONMENT"
	// remember the old value, and set it to something we can check
	oldValue := os.Getenv(envName)
	os.Setenv(envName, "initial")

	s.PatchEnvironment(envName, "new value")
	// Using check to make sure the environment gets set back properly in the test.
	c.Check(os.Getenv(envName), gc.Equals, "new value")

	s.TearDownTest(c)
	c.Check(os.Getenv(envName), gc.Equals, "initial")

	// SetUpTest resets the cleanup stack, this stops the cleanup functions
	// being called again.
	s.SetUpTest(c)
	// explicitly return the envName to the old value
	os.Setenv(envName, oldValue)
}

func (s *cleanupSuite) TestPatchValueInt(c *gc.C) {
	i := 42
	s.PatchValue(&i, 0)
	c.Assert(i, gc.Equals, 0)

	s.TearDownTest(c)
	c.Assert(i, gc.Equals, 42)

	// SetUpTest resets the cleanup stack, this stops the cleanup functions
	// being called again.
	s.SetUpTest(c)
}

func (s *cleanupSuite) TestPatchValueFunction(c *gc.C) {
	function := func() string {
		return "original"
	}

	s.PatchValue(&function, func() string {
		return "patched"
	})
	c.Assert(function(), gc.Equals, "patched")

	s.TearDownTest(c)
	c.Assert(function(), gc.Equals, "original")

	// SetUpTest resets the cleanup stack, this stops the cleanup functions
	// being called again.
	s.SetUpTest(c)
}
