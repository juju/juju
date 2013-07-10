// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package log_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/log"
)

func Test(t *testing.T) {
	TestingT(t)
}

type testWriter struct {
	bytes.Buffer
}

type suite struct {
	writer    *testWriter
	oldWriter loggo.Writer
	oldLevel  loggo.Level
}

var _ = Suite(&suite{})

func (t *testWriter) Write(level loggo.Level, module, filename string, line int, timestamp time.Time, message string) {
	t.Buffer.WriteString(fmt.Sprintf("%s %s %s", level, module, message))
}

func (s *suite) SetUpTest(c *C) {
	var err error
	s.writer = &testWriter{}
	s.oldWriter, s.oldLevel, err = loggo.RemoveWriter("default")
	c.Assert(err, IsNil)
	err = loggo.RegisterWriter("test", s.writer, loggo.TRACE)
	c.Assert(err, IsNil)
	logger := loggo.GetLogger("juju")
	logger.SetLogLevel(loggo.TRACE)
}

func (s *suite) TearDownTest(c *C) {
	_, _, err := loggo.RemoveWriter("test")
	c.Assert(err, IsNil)
	err = loggo.RegisterWriter("default", s.oldWriter, s.oldLevel)
	c.Assert(err, IsNil)
}

func (s *suite) TestLoggerDebug(c *C) {
	input := "Hello World"
	log.Debugf(input)
	c.Assert(s.writer.String(), Equals, "DEBUG juju "+input)
}

func (s *suite) TestInfoLogger(c *C) {
	input := "Hello World"
	log.Infof(input)
	c.Assert(s.writer.String(), Equals, "INFO juju "+input)
}

func (s *suite) TestErrorLogger(c *C) {
	input := "Hello World"
	log.Errorf(input)
	c.Assert(s.writer.String(), Equals, "ERROR juju "+input)
}

func (s *suite) TestWarningLogger(c *C) {
	input := "Hello World"
	log.Warningf(input)
	c.Assert(s.writer.String(), Equals, "WARNING juju "+input)
}

func (s *suite) TestNoticeLogger(c *C) {
	input := "Hello World"
	log.Noticef(input)
	c.Assert(s.writer.String(), Equals, "INFO juju "+input)
}
