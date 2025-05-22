// Copyright 2012-2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
)

// LoggingSuite redirects the juju logger to the test logger
// when embedded in a gocheck suite type.
type LoggingSuite struct {
	mut    sync.RWMutex
	writer *gocheckWriter
	suiteC *tc.C
}

type gocheckWriter struct {
	c interface{ Logf(string, ...any) }
	s *LoggingSuite
}

var logConfig = func() string {
	if cfg := os.Getenv("TEST_LOGGING_CONFIG"); cfg != "" {
		return cfg
	}
	return "DEBUG"
}()

func (w *gocheckWriter) Write(entry loggo.Entry) {
	w.s.mut.RLock()
	defer w.s.mut.RUnlock()
	filename := filepath.Base(entry.Filename)
	w.c.Logf("%s:%d: %s %s %s", filename, entry.Line,
		entry.Level, entry.Module, entry.Message)
}

func (s *LoggingSuite) SetUpSuite(c *tc.C) {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.suiteC = c
	s.writer = &gocheckWriter{
		c: c,
		s: s,
	}
	s.reset(c)
}

func (s *LoggingSuite) reset(c *tc.C) {
	loggo.ResetLogging()
	// Don't use the default writer for the test logging, which
	// means we can still get logging output from tests that
	// replace the default writer.
	_ = loggo.RegisterWriter(loggo.DefaultWriterName, discardWriter{})
	_ = loggo.RegisterWriter("loggingsuite", s.writer)
	err := loggo.ConfigureLoggers(logConfig)
	c.Assert(err, tc.IsNil)
}

func (s *LoggingSuite) TearDownSuite(c *tc.C) {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.writer.c = discardC{}
	loggo.ResetLogging()
}

func (s *LoggingSuite) SetUpTest(c *tc.C) {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.writer.c = c
	s.reset(c)
}

func (s *LoggingSuite) TearDownTest(c *tc.C) {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.writer.c = s.suiteC
	s.reset(c)
}

type discardC struct{}

func (discardC) Logf(string, ...any) {
}

type discardWriter struct{}

func (discardWriter) Write(entry loggo.Entry) {
}

// LoggingCleanupSuite is defined for backward compatibility.
// Do not use this suite in new tests.
type LoggingCleanupSuite struct {
	LoggingSuite
	CleanupSuite
}

func (s *LoggingCleanupSuite) SetUpSuite(c *tc.C) {
	s.CleanupSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
}

func (s *LoggingCleanupSuite) TearDownSuite(c *tc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.CleanupSuite.TearDownSuite(c)
}

func (s *LoggingCleanupSuite) SetUpTest(c *tc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
}

func (s *LoggingCleanupSuite) TearDownTest(c *tc.C) {
	s.CleanupSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}
