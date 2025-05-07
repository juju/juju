// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing_test

import (
	"os"
	"runtime"

	"github.com/juju/tc"

	testing "github.com/juju/juju/internal/testhelpers"
)

type osEnvSuite struct {
	osEnvSuite testing.OsEnvSuite
}

var _ = tc.Suite(&osEnvSuite{})

func (s *osEnvSuite) SetUpSuite(c *tc.C) {
	s.osEnvSuite = testing.OsEnvSuite{}
}

func (s *osEnvSuite) TestOriginalEnvironment(c *tc.C) {
	// The original environment is properly cleaned and restored.
	err := os.Setenv("TESTING_OSENV_ORIGINAL", "original-value")
	c.Assert(err, tc.IsNil)
	s.osEnvSuite.SetUpSuite(c)
	c.Assert(os.Getenv("TESTING_OSENV_ORIGINAL"), tc.Equals, "")
	s.osEnvSuite.TearDownSuite(c)
	// The environment has been restored.
	c.Assert(os.Getenv("TESTING_OSENV_ORIGINAL"), tc.Equals, "original-value")
}

func (s *osEnvSuite) TestTestingEnvironment(c *tc.C) {
	// Environment variables set up by tests are properly removed.
	s.osEnvSuite.SetUpSuite(c)
	s.osEnvSuite.SetUpTest(c)
	err := os.Setenv("TESTING_OSENV_NEW", "new-value")
	c.Assert(err, tc.IsNil)
	s.osEnvSuite.TearDownTest(c)
	s.osEnvSuite.TearDownSuite(c)
	c.Assert(os.Getenv("TESTING_OSENV_NEW"), tc.Equals, "")
}

func (s *osEnvSuite) TestPreservesTestingVariables(c *tc.C) {
	err := os.Setenv("JUJU_MONGOD", "preserved-value")
	c.Assert(err, tc.IsNil)
	s.osEnvSuite.SetUpSuite(c)
	s.osEnvSuite.SetUpTest(c)
	c.Assert(os.Getenv("JUJU_MONGOD"), tc.Equals, "preserved-value")
	c.Assert(err, tc.IsNil)
	s.osEnvSuite.TearDownTest(c)
	s.osEnvSuite.TearDownSuite(c)
	c.Assert(os.Getenv("JUJU_MONGOD"), tc.Equals, "preserved-value")
}

func (s *osEnvSuite) TestRestoresTestingVariables(c *tc.C) {
	os.Clearenv()
	s.osEnvSuite.SetUpSuite(c)
	s.osEnvSuite.SetUpTest(c)
	err := os.Setenv("JUJU_MONGOD", "test-value")
	c.Assert(err, tc.IsNil)
	s.osEnvSuite.TearDownTest(c)
	s.osEnvSuite.TearDownSuite(c)
	c.Assert(os.Getenv("JUJU_MONGOD"), tc.Equals, "")
}

func (s *osEnvSuite) TestWindowsPreservesPath(c *tc.C) {
	if runtime.GOOS != "windows" {
		c.Skip("Windows-specific test case")
	}
	err := os.Setenv("PATH", "/new/path")
	c.Assert(err, tc.IsNil)
	s.osEnvSuite.SetUpSuite(c)
	s.osEnvSuite.SetUpTest(c)
	c.Assert(os.Getenv("PATH"), tc.Equals, "/new/path")
	s.osEnvSuite.TearDownTest(c)
	s.osEnvSuite.TearDownSuite(c)
	c.Assert(os.Getenv("PATH"), tc.Equals, "/new/path")
}

func (s *osEnvSuite) TestWindowsRestoresPath(c *tc.C) {
	if runtime.GOOS != "windows" {
		c.Skip("Windows-specific test case")
	}
	os.Clearenv()
	s.osEnvSuite.SetUpSuite(c)
	s.osEnvSuite.SetUpTest(c)
	err := os.Setenv("PATH", "/test/path")
	c.Assert(err, tc.IsNil)
	s.osEnvSuite.TearDownTest(c)
	s.osEnvSuite.TearDownSuite(c)
	c.Assert(os.Getenv("PATH"), tc.Equals, "")
}
