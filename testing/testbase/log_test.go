// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/testing/testbase"
)

var _ = gc.Suite(&logSuite{})

type logSuite struct {
	testbase.LoggingSuite
}

func (s *logSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	log.Infof("testing-SetUpSuite")
	c.Assert(c.GetTestLog(), gc.Matches, ".*INFO juju testing-SetUpSuite\n")
}

func (s *logSuite) TearDownSuite(c *gc.C) {
	// Unfortunately there's no way of testing that the
	// log output is printed, as the logger is printing
	// a previously set up *gc.C. We print a message
	// anyway so that we can manually verify it.
	log.Infof("testing-TearDownSuite")
}

func (s *logSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	log.Infof("testing-SetUpTest")
	c.Assert(c.GetTestLog(), gc.Matches, ".*INFO juju testing-SetUpTest\n")
}

func (s *logSuite) TearDownTest(c *gc.C) {
	// The same applies here as to TearDownSuite.
	log.Infof("testing-TearDownTest")
	s.LoggingSuite.TearDownTest(c)
}

func (s *logSuite) TestLog(c *gc.C) {
	log.Infof("testing-Test")
	c.Assert(c.GetTestLog(), gc.Matches,
		".*INFO juju testing-SetUpTest\n"+
			".*INFO juju testing-Test\n",
	)
}
