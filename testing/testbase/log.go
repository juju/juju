// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase

import (
	"flag"

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

var logConfig = flag.String("juju.log", "DEBUG", "logging configuration (see http://godoc.org/github.com/juju/loggo#ConfigureLoggers; also accepts a bare log level to configure the log level of the root module")

func (t *LoggingSuite) setUp(c *gc.C) {
	if _, ok := loggo.ParseLevel(*logConfig); ok {
		*logConfig = "<root>=" + *logConfig
	}
	err := loggo.ConfigureLoggers(*logConfig)
	c.Assert(err, gc.IsNil)
}
