// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers_test

import (
	"os"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type cleanupSuite struct {
	testhelpers.CleanupSuite
}

func TestCleanupSuite(t *stdtesting.T) { tc.Run(t, &cleanupSuite{}) }
func (s *cleanupSuite) TestTearDownSuiteEmpty(c *tc.C) {
	// The suite stack is empty initially, check we can tear that down.
	s.TearDownSuite(c)
	s.SetUpSuite(c)
}

func (s *cleanupSuite) TestTearDownTestEmpty(c *tc.C) {
	// The test stack is empty initially, check we can tear that down.
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func (s *cleanupSuite) TestAddCleanup(c *tc.C) {
	order := []string{}
	s.AddCleanup(func(*tc.C) {
		order = append(order, "first")
	})
	s.AddCleanup(func(*tc.C) {
		order = append(order, "second")
	})

	s.TearDownTest(c)
	c.Assert(order, tc.DeepEquals, []string{"second", "first"})

	// SetUpTest resets the cleanup stack, this stops the cleanup functions
	// being called again.
	s.SetUpTest(c)
}

func (s *cleanupSuite) TestPatchEnvironment(c *tc.C) {
	const envName = "TESTING_PATCH_ENVIRONMENT"
	// remember the old value, and set it to something we can check
	oldValue := os.Getenv(envName)
	os.Setenv(envName, "initial")

	s.PatchEnvironment(envName, "new value")
	// Using check to make sure the environment gets set back properly in the test.
	c.Check(os.Getenv(envName), tc.Equals, "new value")

	s.TearDownTest(c)
	c.Check(os.Getenv(envName), tc.Equals, "initial")

	// SetUpTest resets the cleanup stack, this stops the cleanup functions
	// being called again.
	s.SetUpTest(c)
	// explicitly return the envName to the old value
	os.Setenv(envName, oldValue)
}

func (s *cleanupSuite) TestPatchValueInt(c *tc.C) {
	i := 42
	s.PatchValue(&i, 0)
	c.Assert(i, tc.Equals, 0)

	s.TearDownTest(c)
	c.Assert(i, tc.Equals, 42)

	// SetUpTest resets the cleanup stack, this stops the cleanup functions
	// being called again.
	s.SetUpTest(c)
}

func (s *cleanupSuite) TestPatchValueFunction(c *tc.C) {
	function := func() string {
		return "original"
	}

	s.PatchValue(&function, func() string {
		return "patched"
	})
	c.Assert(function(), tc.Equals, "patched")

	s.TearDownTest(c)
	c.Assert(function(), tc.Equals, "original")

	// SetUpTest resets the cleanup stack, this stops the cleanup functions
	// being called again.
	s.SetUpTest(c)
}

// noopCleanup is a simple function that does nothing that can be passed to
// AddCleanup
func noopCleanup(*tc.C) {
}

func (s cleanupSuite) TestAddCleanupPanicIfUnsafe(c *tc.C) {
	// It is unsafe to call AddCleanup when the test itself is not a
	// pointer receiver, because AddCleanup modifies the s.testStack
	// attribute, but in a non-pointer receiver, that object is lost when
	// the Test function returns.
	// This Test must, itself, be a non pointer receiver to trigger this
	c.Assert(func() { s.AddCleanup(noopCleanup) },
		tc.PanicMatches,
		"unsafe to call AddCleanup from non pointer receiver test")
}

type cleanupSuiteAndTestLifetimes struct {
}

func TestCleanupSuiteAndTestLifetimes(t *stdtesting.T) {
	tc.Run(t, &cleanupSuiteAndTestLifetimes{})
}
func (s *cleanupSuiteAndTestLifetimes) TestAddCleanupBeforeSetUpSuite(c *tc.C) {
	suite := &testhelpers.CleanupSuite{}
	c.Assert(func() { suite.AddCleanup(noopCleanup) },
		tc.PanicMatches,
		"unsafe to call AddCleanup before SetUpSuite")
	suite.SetUpSuite(c)
	suite.SetUpTest(c)
	suite.TearDownTest(c)
	suite.TearDownSuite(c)
}

func (s *cleanupSuiteAndTestLifetimes) TestAddCleanupAfterTearDownSuite(c *tc.C) {
	suite := &testhelpers.CleanupSuite{}
	suite.SetUpSuite(c)
	suite.SetUpTest(c)
	suite.TearDownTest(c)
	suite.TearDownSuite(c)
	c.Assert(func() { suite.AddCleanup(noopCleanup) },
		tc.PanicMatches,
		"unsafe to call AddCleanup after TearDownSuite")
}

func (s *cleanupSuiteAndTestLifetimes) TestAddCleanupMixedSuiteAndTest(c *tc.C) {
	calls := []string{}
	suite := &testhelpers.CleanupSuite{}
	suite.SetUpSuite(c)
	suite.AddCleanup(func(*tc.C) { calls = append(calls, "before SetUpTest") })
	suite.SetUpTest(c)
	suite.AddCleanup(func(*tc.C) { calls = append(calls, "during Test1") })
	suite.TearDownTest(c)
	c.Check(calls, tc.DeepEquals, []string{
		"during Test1",
	})
	c.Assert(func() { suite.AddCleanup(noopCleanup) },
		tc.PanicMatches,
		"unsafe to call AddCleanup after a test has been torn down"+
			" before a new test has been set up"+
			" \\(Suite level changes only make sense before first test is run\\)")
	suite.SetUpTest(c)
	suite.AddCleanup(func(*tc.C) { calls = append(calls, "during Test2") })
	suite.TearDownTest(c)
	c.Check(calls, tc.DeepEquals, []string{
		"during Test1",
		"during Test2",
	})
	suite.TearDownSuite(c)
	c.Check(calls, tc.DeepEquals, []string{
		"during Test1",
		"during Test2",
		"before SetUpTest",
	})
}
