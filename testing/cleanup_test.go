package testing_test

import (
	"os"

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

	// and to avoid calling them twice, clear out the stack
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

	// and to avoid calling them twice, clear out the stack
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

	// and to avoid calling them twice, clear out the stack
	s.SetUpTest(c)
}
