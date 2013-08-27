// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/loggo"
)

// LoggingSuite redirects the juju logger to the test logger
// when embedded in a gocheck suite type.
type LoggingSuite struct {
	restoreLog func()
}

type gocheckWriter struct {
	c *C
}

func (w *gocheckWriter) Write(level loggo.Level, module, filename string, line int, timestamp time.Time, message string) {
	// Magic calldepth value...
	w.c.Output(3, fmt.Sprintf("%s %s %s", level, module, message))
}

func (t *LoggingSuite) SetUpSuite(c *C)    {
	loggo.ResetWriters()
	loggo.ReplaceDefaultWriter(&gocheckWriter{c})
	loggo.ResetLoggers()
	loggo.GetLogger("juju").SetLogLevel(loggo.DEBUG)
}

func (t *LoggingSuite) TearDownSuite(c *C) {
	loggo.ResetLoggers()
	loggo.ResetWriters()
}

func (t *LoggingSuite) SetUpTest(c *C) {
}

func (t *LoggingSuite) TearDownTest(c *C) {
}
