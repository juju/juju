// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package log_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/log"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type testWriter struct {
	bytes.Buffer
}

type suite struct {
	writer    *testWriter
	oldWriter loggo.Writer
	oldLevel  loggo.Level
}

var _ = gc.Suite(&suite{})

func (t *testWriter) Write(level loggo.Level, module, filename string, line int, timestamp time.Time, message string) {
	t.Buffer.WriteString(fmt.Sprintf("%s %s %s", level, module, message))
}

func (s *suite) SetUpTest(c *gc.C) {
	var err error
	s.writer = &testWriter{}
	s.oldWriter, s.oldLevel, err = loggo.RemoveWriter("default")
	c.Assert(err, gc.IsNil)
	err = loggo.RegisterWriter("test", s.writer, loggo.TRACE)
	c.Assert(err, gc.IsNil)
	logger := loggo.GetLogger("juju")
	logger.SetLogLevel(loggo.TRACE)
}

func (s *suite) TearDownTest(c *gc.C) {
	_, _, err := loggo.RemoveWriter("test")
	c.Assert(err, gc.IsNil)
	err = loggo.RegisterWriter("default", s.oldWriter, s.oldLevel)
	c.Assert(err, gc.IsNil)
}

func (s *suite) TestLoggerDebug(c *gc.C) {
	input := "Hello World"
	log.Debugf(input)
	c.Assert(s.writer.String(), gc.Equals, "DEBUG juju "+input)
}

func (s *suite) TestInfoLogger(c *gc.C) {
	input := "Hello World"
	log.Infof(input)
	c.Assert(s.writer.String(), gc.Equals, "INFO juju "+input)
}

func (s *suite) TestErrorLogger(c *gc.C) {
	input := "Hello World"
	log.Errorf(input)
	c.Assert(s.writer.String(), gc.Equals, "ERROR juju "+input)
}

func (s *suite) TestWarningLogger(c *gc.C) {
	input := "Hello World"
	log.Warningf(input)
	c.Assert(s.writer.String(), gc.Equals, "WARNING juju "+input)
}

func (s *suite) TestNoticeLogger(c *gc.C) {
	input := "Hello World"
	log.Noticef(input)
	c.Assert(s.writer.String(), gc.Equals, "INFO juju "+input)
}
