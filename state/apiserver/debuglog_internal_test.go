// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This is an internal package test.

package apiserver

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

type debugInternalSuite struct {
	testing.BaseSuite
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

func checkIncludeEntity(logValue string, agent ...string) bool {
	stream := &logStream{includeEntity: agent}
	line := &logLine{agent: logValue}
	return stream.checkIncludeEntity(line)
}

func (s *debugInternalSuite) TestCheckIncludeEntity(c *gc.C) {
	c.Check(checkIncludeEntity("machine-0"), jc.IsTrue)
	c.Check(checkIncludeEntity("machine-0", "machine-0"), jc.IsTrue)
	c.Check(checkIncludeEntity("machine-1", "machine-0"), jc.IsFalse)
	c.Check(checkIncludeEntity("machine-1", "machine-0", "machine-1"), jc.IsTrue)
	c.Check(checkIncludeEntity("machine-0-lxc-0", "machine-0"), jc.IsFalse)
	c.Check(checkIncludeEntity("machine-0-lxc-0", "machine-0*"), jc.IsTrue)
	c.Check(checkIncludeEntity("machine-0-lxc-0", "machine-0-lxc-*"), jc.IsTrue)
}

func checkIncludeModule(logValue string, module ...string) bool {
	stream := &logStream{includeModule: module}
	line := &logLine{module: logValue}
	return stream.checkIncludeModule(line)
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

func checkExcludeEntity(logValue string, agent ...string) bool {
	stream := &logStream{excludeEntity: agent}
	line := &logLine{agent: logValue}
	return stream.exclude(line)
}

func (s *debugInternalSuite) TestCheckExcludeEntity(c *gc.C) {
	c.Check(checkExcludeEntity("machine-0"), jc.IsFalse)
	c.Check(checkExcludeEntity("machine-0", "machine-0"), jc.IsTrue)
	c.Check(checkExcludeEntity("machine-1", "machine-0"), jc.IsFalse)
	c.Check(checkExcludeEntity("machine-1", "machine-0", "machine-1"), jc.IsTrue)
	c.Check(checkExcludeEntity("machine-0-lxc-0", "machine-0"), jc.IsFalse)
	c.Check(checkExcludeEntity("machine-0-lxc-0", "machine-0*"), jc.IsTrue)
	c.Check(checkExcludeEntity("machine-0-lxc-0", "machine-0-lxc-*"), jc.IsTrue)
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
		includeEntity: []string{"machine-0", "unit-mysql*"},
		includeModule: []string{"juju"},
		excludeEntity: []string{"unit-mysql-2"},
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

func (s *debugInternalSuite) TestCountedFilterLineWithLimit(c *gc.C) {
	stream := &logStream{
		filterLevel: loggo.INFO,
		maxLines:    5,
	}
	line := []byte("machine-0: date time WARNING juju")
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsFalse)
	c.Check(stream.countedFilterLine(line), jc.IsFalse)
}

func (s *debugInternalSuite) testStreamInternal(c *gc.C, fromTheStart bool, backlog, maxLines uint, expected, errMatch string) {

	dir := c.MkDir()
	logPath := filepath.Join(dir, "logfile.txt")
	logFile, err := os.Create(logPath)
	c.Assert(err, gc.IsNil)
	defer logFile.Close()
	logFileReader, err := os.Open(logPath)
	c.Assert(err, gc.IsNil)
	defer logFileReader.Close()

	logFile.WriteString(`line 1
line 2
line 3
`)
	stream := &logStream{
		fromTheStart: fromTheStart,
		backlog:      backlog,
		maxLines:     maxLines,
	}
	err = stream.positionLogFile(logFileReader)
	c.Assert(err, gc.IsNil)
	output := &bytes.Buffer{}
	stream.start(logFileReader, output)

	go func() {
		defer stream.tomb.Done()
		stream.tomb.Kill(stream.loop())
	}()

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

	stream.logTailer.Stop()

	err = stream.tomb.Wait()
	if errMatch == "" {
		c.Assert(err, gc.IsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, errMatch)
	}
}

func (s *debugInternalSuite) TestLogStreamLoopFromTheStart(c *gc.C) {
	expected := `line 1
line 2
line 3
line 4
line 5
`
	s.testStreamInternal(c, true, 0, 0, expected, "")
}

func (s *debugInternalSuite) TestLogStreamLoopFromTheStartMaxLines(c *gc.C) {
	expected := `line 1
line 2
line 3
`
	s.testStreamInternal(c, true, 0, 3, expected, "max lines reached")
}

func (s *debugInternalSuite) TestLogStreamLoopJustTail(c *gc.C) {
	expected := `line 4
line 5
`
	s.testStreamInternal(c, false, 0, 0, expected, "")
}

func (s *debugInternalSuite) TestLogStreamLoopBackOneLimitTwo(c *gc.C) {
	expected := `line 3
line 4
`
	s.testStreamInternal(c, false, 1, 2, expected, "max lines reached")
}

func (s *debugInternalSuite) TestLogStreamLoopTailMaxLinesNotYetReached(c *gc.C) {
	expected := `line 4
line 5
`
	s.testStreamInternal(c, false, 0, 3, expected, "")
}

func assertStreamParams(c *gc.C, obtained, expected *logStream) {
	c.Check(obtained.includeEntity, jc.DeepEquals, expected.includeEntity)
	c.Check(obtained.includeModule, jc.DeepEquals, expected.includeModule)
	c.Check(obtained.excludeEntity, jc.DeepEquals, expected.excludeEntity)
	c.Check(obtained.excludeModule, jc.DeepEquals, expected.excludeModule)
	c.Check(obtained.maxLines, gc.Equals, expected.maxLines)
	c.Check(obtained.fromTheStart, gc.Equals, expected.fromTheStart)
	c.Check(obtained.filterLevel, gc.Equals, expected.filterLevel)
	c.Check(obtained.backlog, gc.Equals, expected.backlog)
}

func (s *debugInternalSuite) TestNewLogStream(c *gc.C) {
	obtained, err := newLogStream(nil)
	c.Assert(err, gc.IsNil)
	assertStreamParams(c, obtained, &logStream{})

	values := url.Values{
		"includeEntity": []string{"machine-1*", "machine-2"},
		"includeModule": []string{"juju", "unit"},
		"excludeEntity": []string{"machine-1-lxc*"},
		"excludeModule": []string{"juju.provisioner"},
		"maxLines":      []string{"300"},
		"backlog":       []string{"100"},
		"level":         []string{"INFO"},
		// OK, just a little nonsense
		"replay": []string{"true"},
	}
	expected := &logStream{
		includeEntity: []string{"machine-1*", "machine-2"},
		includeModule: []string{"juju", "unit"},
		excludeEntity: []string{"machine-1-lxc*"},
		excludeModule: []string{"juju.provisioner"},
		maxLines:      300,
		backlog:       100,
		filterLevel:   loggo.INFO,
		fromTheStart:  true,
	}
	obtained, err = newLogStream(values)
	c.Assert(err, gc.IsNil)
	assertStreamParams(c, obtained, expected)

	_, err = newLogStream(url.Values{"maxLines": []string{"foo"}})
	c.Assert(err, gc.ErrorMatches, `maxLines value "foo" is not a valid unsigned number`)

	_, err = newLogStream(url.Values{"backlog": []string{"foo"}})
	c.Assert(err, gc.ErrorMatches, `backlog value "foo" is not a valid unsigned number`)

	_, err = newLogStream(url.Values{"replay": []string{"foo"}})
	c.Assert(err, gc.ErrorMatches, `replay value "foo" is not a valid boolean`)

	_, err = newLogStream(url.Values{"level": []string{"foo"}})
	c.Assert(err, gc.ErrorMatches, `level value "foo" is not one of "TRACE", "DEBUG", "INFO", "WARNING", "ERROR"`)
}
