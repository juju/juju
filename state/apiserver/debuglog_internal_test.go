// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This is an internal package test.

package apiserver

import (
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type debugInternalSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&debugInternalSuite{})

func (s *debugInternalSuite) TestParseLogLine(c *gc.C) {
	line := "machine-0: 2014-03-24 22:34:25 INFO juju.cmd.jujud machine.go:127 machine agent machine-0 start (1.17.7.1-trusty-amd64 [gc])"
	logLine := parseLogLine(line)
	c.Assert(logLine.line, gc.Equals, line)
	c.Assert(logLine.agent, gc.Equals, "machine-0")
	c.Assert(logLine.level, gc.Equals, loggo.INFO)
	c.Assert(logLine.module, gc.Equals, "juju.cmd.jujud")
}

func (s *debugInternalSuite) TestParseLogLineMachineMultiline(c *gc.C) {
	line := "machine-1: continuation line"
	logLine := parseLogLine(line)
	c.Assert(logLine.line, gc.Equals, line)
	c.Assert(logLine.agent, gc.Equals, "machine-1")
	c.Assert(logLine.level, gc.Equals, loggo.UNSPECIFIED)
	c.Assert(logLine.module, gc.Equals, "")
}

func (s *debugInternalSuite) TestParseLogLineInvalid(c *gc.C) {
	line := "not a full line"
	logLine := parseLogLine(line)
	c.Assert(logLine.line, gc.Equals, line)
	c.Assert(logLine.agent, gc.Equals, "")
	c.Assert(logLine.level, gc.Equals, loggo.UNSPECIFIED)
	c.Assert(logLine.module, gc.Equals, "")
}

func checkLevel(logValue, streamValue loggo.Level) bool {
	stream := &logStream{}
	if streamValue != loggo.UNSPECIFIED {
		stream.filterLevel = &streamValue
	}
	line := &logLine{level: logValue}
	return stream.checkLevel(line)
}

func (s *debugInternalSuite) TestCheckLevel(c *gc.C) {
	c.Check(checkLevel(loggo.UNSPECIFIED, loggo.UNSPECIFIED), jc.IsTrue)
	c.Check(checkLevel(loggo.TRACE, loggo.UNSPECIFIED), jc.IsTrue)
	c.Check(checkLevel(loggo.DEBUG, loggo.UNSPECIFIED), jc.IsTrue)
	c.Check(checkLevel(loggo.INFO, loggo.UNSPECIFIED), jc.IsTrue)
	c.Check(checkLevel(loggo.WARNING, loggo.UNSPECIFIED), jc.IsTrue)
	c.Check(checkLevel(loggo.ERROR, loggo.UNSPECIFIED), jc.IsTrue)
	c.Check(checkLevel(loggo.CRITICAL, loggo.UNSPECIFIED), jc.IsTrue)

	c.Check(checkLevel(loggo.UNSPECIFIED, loggo.TRACE), jc.IsFalse)
	c.Check(checkLevel(loggo.TRACE, loggo.TRACE), jc.IsTrue)
	c.Check(checkLevel(loggo.DEBUG, loggo.TRACE), jc.IsTrue)
	c.Check(checkLevel(loggo.INFO, loggo.TRACE), jc.IsTrue)
	c.Check(checkLevel(loggo.WARNING, loggo.TRACE), jc.IsTrue)
	c.Check(checkLevel(loggo.ERROR, loggo.TRACE), jc.IsTrue)
	c.Check(checkLevel(loggo.CRITICAL, loggo.TRACE), jc.IsTrue)

	c.Check(checkLevel(loggo.UNSPECIFIED, loggo.INFO), jc.IsFalse)
	c.Check(checkLevel(loggo.TRACE, loggo.INFO), jc.IsFalse)
	c.Check(checkLevel(loggo.DEBUG, loggo.INFO), jc.IsFalse)
	c.Check(checkLevel(loggo.INFO, loggo.INFO), jc.IsTrue)
	c.Check(checkLevel(loggo.WARNING, loggo.INFO), jc.IsTrue)
	c.Check(checkLevel(loggo.ERROR, loggo.INFO), jc.IsTrue)
	c.Check(checkLevel(loggo.CRITICAL, loggo.INFO), jc.IsTrue)
}

func checkIncludeAgent(logValue string, includeAgent ...string) bool {
	stream := &logStream{includeAgent: includeAgent}
	line := &logLine{agent: logValue}
	return stream.include(line)
}

func (s *debugInternalSuite) TestCheckIncludeAgent(c *gc.C) {
	c.Check(checkIncludeAgent("machine-0"), jc.IsTrue)
	c.Check(checkIncludeAgent("machine-0", "machine-0"), jc.IsTrue)
	c.Check(checkIncludeAgent("machine-1", "machine-0"), jc.IsFalse)
	c.Check(checkIncludeAgent("machine-1", "machine-0", "machine-1"), jc.IsTrue)
	c.Check(checkIncludeAgent("machine-0-lxc-0", "machine-0"), jc.IsFalse)
	c.Check(checkIncludeAgent("machine-0-lxc-0", "machine-0*"), jc.IsTrue)
	c.Check(checkIncludeAgent("machine-0-lxc-0", "machine-0-lxc-*"), jc.IsTrue)
}

func checkIncludeModule(logValue string, module ...string) bool {
	stream := &logStream{includeModule: module}
	line := &logLine{module: logValue}
	return stream.include(line)
}

func (s *debugInternalSuite) TestCheckIncludeModule(c *gc.C) {
	c.Check(checkIncludeModule("juju"), jc.IsTrue)
	c.Check(checkIncludeModule("juju", "juju"), jc.IsTrue)
	c.Check(checkIncludeModule("juju", "juju.environ"), jc.IsFalse)
	c.Check(checkIncludeModule("juju.provisioner", "juju"), jc.IsTrue)
	c.Check(checkIncludeModule("juju.provisioner", "juju*"), jc.IsFalse)
	c.Check(checkIncludeModule("juju.provisioner", "juju.environ"), jc.IsFalse)
	c.Check(checkIncludeModule("unit.mysql/1", "juju", "unit"), jc.IsTrue)
}
