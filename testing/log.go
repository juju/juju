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

func (t *LoggingSuite) SetUpSuite(c *C)    {}
func (t *LoggingSuite) TearDownSuite(c *C) {}

func (t *LoggingSuite) SetUpTest(c *C) {
	oldWriter, oldLevel, err := loggo.RemoveWriter("default")
	c.Assert(err, IsNil)
	err = loggo.RegisterWriter("test", &gocheckWriter{c}, loggo.TRACE)
	c.Assert(err, IsNil)
	loggo.ResetLogging()

	t.restoreLog = func() {
		_, _, err := loggo.RemoveWriter("test")
		c.Assert(err, IsNil)
		err = loggo.RegisterWriter("default", oldWriter, oldLevel)
		c.Assert(err, IsNil)
		loggo.ResetLogging()
	}
}

func (t *LoggingSuite) TearDownTest(c *C) {
	t.restoreLog()
}
