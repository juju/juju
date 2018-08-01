// Copyright 2012-2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/loggo"
	gc "gopkg.in/check.v1"
)

var logLocation = flag.Bool("loggo.location", false, "Also log the location of the loggo call")

// LoggingSuite redirects the juju logger to the test logger
// when embedded in a gocheck suite type.
type LoggingSuite struct{}

type gocheckWriter struct {
	c *gc.C
}

var logConfig = func() string {
	if cfg := os.Getenv("TEST_LOGGING_CONFIG"); cfg != "" {
		return cfg
	}
	return "DEBUG"
}()

func (w *gocheckWriter) Write(entry loggo.Entry) {
	filename := filepath.Base(entry.Filename)
	var message string
	if *logLocation {
		message = fmt.Sprintf("%s %s %s:%d %s", entry.Level, entry.Module, filename, entry.Line, entry.Message)
	} else {
		message = fmt.Sprintf("%s %s %s", entry.Level, entry.Module, entry.Message)
	}
	// Magic calldepth value...
	// The value says "how far up the call stack do we go to find the location".
	// It is used to match the standard library log function, and isn't actually
	// used by gocheck.
	w.c.Output(3, message)
}

func (s *LoggingSuite) SetUpSuite(c *gc.C) {
	s.setUp(c)
}

func (s *LoggingSuite) TearDownSuite(c *gc.C) {
	loggo.ResetLogging()
}

func (s *LoggingSuite) SetUpTest(c *gc.C) {
	s.setUp(c)
}

func (s *LoggingSuite) TearDownTest(c *gc.C) {
}

type discardWriter struct{}

func (discardWriter) Write(entry loggo.Entry) {
}

func (s *LoggingSuite) setUp(c *gc.C) {
	loggo.ResetLogging()
	// Don't use the default writer for the test logging, which
	// means we can still get logging output from tests that
	// replace the default writer.
	loggo.RegisterWriter(loggo.DefaultWriterName, discardWriter{})
	loggo.RegisterWriter("loggingsuite", &gocheckWriter{c})
	err := loggo.ConfigureLoggers(logConfig)
	c.Assert(err, gc.IsNil)
}

// LoggingCleanupSuite is defined for backward compatibility.
// Do not use this suite in new tests.
type LoggingCleanupSuite struct {
	LoggingSuite
	CleanupSuite
}

func (s *LoggingCleanupSuite) SetUpSuite(c *gc.C) {
	s.CleanupSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
}

func (s *LoggingCleanupSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.CleanupSuite.TearDownSuite(c)
}

func (s *LoggingCleanupSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
}

func (s *LoggingCleanupSuite) TearDownTest(c *gc.C) {
	s.CleanupSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}
