// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	gc "gopkg.in/check.v1"
)

// IsolationSuite isolates the tests from the underlaying system environment,
// sets up test logging and exposes cleanup facilities.
type IsolationSuite struct {
	OsEnvSuite
	CleanupSuite
	LoggingSuite
}

func (s *IsolationSuite) SetUpSuite(c *gc.C) {
	s.OsEnvSuite.SetUpSuite(c)
	s.CleanupSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
}

func (s *IsolationSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.CleanupSuite.TearDownSuite(c)
	s.OsEnvSuite.TearDownSuite(c)
}

func (s *IsolationSuite) SetUpTest(c *gc.C) {
	s.OsEnvSuite.SetUpTest(c)
	s.CleanupSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
}

func (s *IsolationSuite) TearDownTest(c *gc.C) {
	s.LoggingSuite.TearDownTest(c)
	s.CleanupSuite.TearDownTest(c)
	s.OsEnvSuite.TearDownTest(c)
}
