// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This is an internal package test.

package apiserver

import (
	"bytes"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
	"time"

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
		stream.filterLevel = streamValue
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

func checkIncludeAgent(logValue string, agent ...string) bool {
	stream := &logStream{includeAgent: agent}
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

func checkExcludeAgent(logValue string, agent ...string) bool {
	stream := &logStream{excludeAgent: agent}
	line := &logLine{agent: logValue}
	return stream.exclude(line)
}

func (s *debugInternalSuite) TestCheckExcludeAgent(c *gc.C) {
	c.Check(checkExcludeAgent("machine-0"), jc.IsFalse)
	c.Check(checkExcludeAgent("machine-0", "machine-0"), jc.IsTrue)
	c.Check(checkExcludeAgent("machine-1", "machine-0"), jc.IsFalse)
	c.Check(checkExcludeAgent("machine-1", "machine-0", "machine-1"), jc.IsTrue)
	c.Check(checkExcludeAgent("machine-0-lxc-0", "machine-0"), jc.IsFalse)
	c.Check(checkExcludeAgent("machine-0-lxc-0", "machine-0*"), jc.IsTrue)
	c.Check(checkExcludeAgent("machine-0-lxc-0", "machine-0-lxc-*"), jc.IsTrue)
}

func checkExcludeModule(logValue string, module ...string) bool {
	stream := &logStream{excludeModule: module}
	line := &logLine{module: logValue}
	return stream.exclude(line)
}

func (s *debugInternalSuite) TestCheckExcludeModule(c *gc.C) {
	c.Check(checkExcludeModule("juju"), jc.IsFalse)
	c.Check(checkExcludeModule("juju", "juju"), jc.IsTrue)
	c.Check(checkExcludeModule("juju", "juju.environ"), jc.IsFalse)
	c.Check(checkExcludeModule("juju.provisioner", "juju"), jc.IsTrue)
	c.Check(checkExcludeModule("juju.provisioner", "juju*"), jc.IsFalse)
	c.Check(checkExcludeModule("juju.provisioner", "juju.environ"), jc.IsFalse)
	c.Check(checkExcludeModule("unit.mysql/1", "juju", "unit"), jc.IsTrue)
}

func (s *debugInternalSuite) TestFilterLine(c *gc.C) {
	stream := &logStream{
		filterLevel:   loggo.INFO,
		includeAgent:  []string{"machine-0", "unit-mysql*"},
		excludeAgent:  []string{"unit-mysql-2"},
		excludeModule: []string{"juju.foo"},
	}
	c.Check(stream.filterLine([]byte(
		"machine-0: date time WARNING juju")), jc.IsTrue)
	c.Check(stream.filterLine([]byte(
		"machine-1: date time WARNING juju")), jc.IsFalse)
	c.Check(stream.filterLine([]byte(
		"unit-mysql-0: date time WARNING juju")), jc.IsTrue)
	c.Check(stream.filterLine([]byte(
		"unit-mysql-1: date time WARNING juju")), jc.IsTrue)
	c.Check(stream.filterLine([]byte(
		"unit-mysql-2: date time WARNING juju")), jc.IsFalse)
	c.Check(stream.filterLine([]byte(
		"unit-wordpress-0: date time WARNING juju")), jc.IsFalse)
	c.Check(stream.filterLine([]byte(
		"machine-0: date time DEBUG juju")), jc.IsFalse)
	c.Check(stream.filterLine([]byte(
		"machine-0: date time WARNING juju.foo.bar")), jc.IsFalse)
}

func (s *debugInternalSuite) TestFilterLineWithLimit(c *gc.C) {
	stream := &logStream{
		filterLevel: loggo.INFO,
		maxLines:    5,
	}
	line := []byte("machine-0: date time WARNING juju")
	c.Check(stream.filterLine(line), jc.IsTrue)
	c.Check(stream.filterLine(line), jc.IsTrue)
	c.Check(stream.filterLine(line), jc.IsTrue)
	c.Check(stream.filterLine(line), jc.IsTrue)
	c.Check(stream.filterLine(line), jc.IsTrue)
	c.Check(stream.filterLine(line), jc.IsFalse)
	c.Check(stream.filterLine(line), jc.IsFalse)
}

func (s *debugInternalSuite) testStreamInternal(c *gc.C, fromTheStart bool, maxLines uint, expected string) {

	dir := c.MkDir()
	logPath := filepath.Join(dir, "logfile.txt")
	logFile, err := os.Create(logPath)
	c.Assert(err, gc.IsNil)
	logFileReader, err := os.Open(logPath)
	c.Assert(err, gc.IsNil)
	defer logFile.Close()

	logFile.WriteString(`line 1
line 2
line 3
`)
	stream := &logStream{fromTheStart: fromTheStart, maxLines: maxLines}
	output := &bytes.Buffer{}
	stream.init(logFileReader, output)

	tailingStarted := make(chan struct{})
	stream.logTailer.StartedTailing = func() {
		close(tailingStarted)
	}

	go func() {
		defer stream.tomb.Done()
		stream.tomb.Kill(stream.loop())
	}()
	// wait for the tailer to have started tailing before writing more
	<-tailingStarted

	logFile.WriteString("line 4\n")
	logFile.WriteString("line 5\n")

	timeout := time.After(testing.LongWait)
	for output.String() != expected {
		select {
		case <-time.After(testing.ShortWait):
			// do nothing
		case <-timeout:
			c.Fatalf("expected data didn't arrive:\n\tobtained: %#v\n\texpected: %#v", output.String(), expected)
		}
	}

	logFile.Close()

	//err = stream.tomb.Wait()
	//c.Assert(err, gc.IsNil)
}

func (s *debugInternalSuite) TestLogStreamLoopFromTheStart(c *gc.C) {
	expected := `line 1
line 2
line 3
line 4
line 5
`
	s.testStreamInternal(c, true, 0, expected)
}

func (s *debugInternalSuite) TestLogStreamLoopFromTheStartMaxLines(c *gc.C) {
	expected := `line 1
line 2
line 3
`
	s.testStreamInternal(c, true, 3, expected)
}

func (s *debugInternalSuite) TestLogStreamLoopJustTail(c *gc.C) {
	expected := `line 4
line 5
`
	s.testStreamInternal(c, false, 0, expected)
}
