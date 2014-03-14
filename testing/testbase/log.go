// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase

import (
	"github.com/juju/loggo"
	"github.com/juju/testing/logging"
	gc "launchpad.net/gocheck"
)

// LoggingSuite redirects the juju logger to the test logger
// when embedded in a gocheck suite type.
type LoggingSuite struct {
	logging.LoggingSuite
}

func (t *LoggingSuite) SetUpSuite(c *gc.C) {
	t.LoggingSuite.SetUpSuite(c)
	t.setUp(c)
}

func (t *LoggingSuite) SetUpTest(c *gc.C) {
	t.LoggingSuite.SetUpTest(c)
	t.PatchEnvironment("JUJU_LOGGING_CONFIG", "")
	t.setUp(c)
}

func (t *LoggingSuite) setUp(c *gc.C) {
	loggo.GetLogger("juju").SetLogLevel(loggo.DEBUG)
	loggo.GetLogger("unit").SetLogLevel(loggo.DEBUG)
}
