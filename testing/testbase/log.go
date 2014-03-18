// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase

import (
	"flag"
	"fmt"
	"time"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"
)

// LoggingSuite redirects the juju logger to the test logger
// when embedded in a gocheck suite type.
type LoggingSuite struct {
	CleanupSuite
}

type gocheckWriter struct {
	c *gc.C
}

func (w *gocheckWriter) Write(level loggo.Level, module, filename string, line int, timestamp time.Time, message string) {
	// Magic calldepth value...
	w.c.Output(3, fmt.Sprintf("%s %s %s", level, module, message))
}

func (t *LoggingSuite) SetUpSuite(c *gc.C) {
	t.CleanupSuite.SetUpSuite(c)
	t.setUp(c)
	t.AddSuiteCleanup(func(*gc.C) {
		loggo.ResetLoggers()
		loggo.ResetWriters()
	})
}

func (t *LoggingSuite) SetUpTest(c *gc.C) {
	t.CleanupSuite.SetUpTest(c)
	t.PatchEnvironment("JUJU_LOGGING_CONFIG", "")
	t.setUp(c)
}

var logConfig = flag.String("juju.log", "DEBUG", "logging configuration (see http://godoc.org/github.com/juju/loggo#ConfigureLoggers; also accepts a bare log level to configure the log level of the root module")

func (t *LoggingSuite) setUp(c *gc.C) {
	loggo.ResetWriters()
	loggo.ReplaceDefaultWriter(&gocheckWriter{c})
	loggo.ResetLoggers()
	if _, ok := loggo.ParseLevel(*logConfig); ok {
		*logConfig = "<root>=" + *logConfig
	}
	err := loggo.ConfigureLoggers(*logConfig)
	c.Assert(err, gc.IsNil)
}
