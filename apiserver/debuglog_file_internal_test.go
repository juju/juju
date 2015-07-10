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
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type debugLogFileIntSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&debugLogFileIntSuite{})

func (s *debugLogFileIntSuite) TestParseLogLine(c *gc.C) {
	line := "machine-0: 2014-03-24 22:34:25 INFO juju.cmd.jujud machine.go:127 machine agent machine-0 start (1.17.7.1-trusty-amd64 [gc])"
	logLine := parseLogLine(line)
	c.Assert(logLine.line, gc.Equals, line)
	c.Assert(logLine.agentTag, gc.Equals, "machine-0")
	c.Assert(logLine.level, gc.Equals, loggo.INFO)
	c.Assert(logLine.module, gc.Equals, "juju.cmd.jujud")
}

func (s *debugLogFileIntSuite) TestParseLogLineMachineMultiline(c *gc.C) {
	line := "machine-1: continuation line"
	logLine := parseLogLine(line)
	c.Assert(logLine.line, gc.Equals, line)
	c.Assert(logLine.agentTag, gc.Equals, "machine-1")
	c.Assert(logLine.level, gc.Equals, loggo.UNSPECIFIED)
	c.Assert(logLine.module, gc.Equals, "")
}

func (s *debugLogFileIntSuite) TestParseLogLineInvalid(c *gc.C) {
	line := "not a full line"
	logLine := parseLogLine(line)
	c.Assert(logLine.line, gc.Equals, line)
	c.Assert(logLine.agentTag, gc.Equals, "")
	c.Assert(logLine.level, gc.Equals, loggo.UNSPECIFIED)
	c.Assert(logLine.module, gc.Equals, "")
}

func checkLevel(logValue, streamValue loggo.Level) bool {
	line := &logFileLine{level: logValue}
	params := debugLogParams{}
	if streamValue != loggo.UNSPECIFIED {
		params.filterLevel = streamValue
	}
	return newLogFileStream(&params).checkLevel(line)
}

func (s *debugLogFileIntSuite) TestCheckLevel(c *gc.C) {
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
	stream := newLogFileStream(&debugLogParams{
		includeEntity: agent,
	})
	line := &logFileLine{agentTag: logValue}
	return stream.checkIncludeEntity(line)
}

func (s *debugLogFileIntSuite) TestCheckIncludeEntity(c *gc.C) {
	c.Check(checkIncludeEntity("machine-0"), jc.IsTrue)
	c.Check(checkIncludeEntity("machine-0", "machine-0"), jc.IsTrue)
	c.Check(checkIncludeEntity("machine-1", "machine-0"), jc.IsFalse)
	c.Check(checkIncludeEntity("machine-1", "machine-0", "machine-1"), jc.IsTrue)
	c.Check(checkIncludeEntity("machine-0-lxc-0", "machine-0"), jc.IsFalse)
	c.Check(checkIncludeEntity("machine-0-lxc-0", "machine-0*"), jc.IsTrue)
	c.Check(checkIncludeEntity("machine-0-lxc-0", "machine-0-lxc-*"), jc.IsTrue)
}

func checkIncludeModule(logValue string, module ...string) bool {
	stream := newLogFileStream(&debugLogParams{
		includeModule: module,
	})
	line := &logFileLine{module: logValue}
	return stream.checkIncludeModule(line)
}

func (s *debugLogFileIntSuite) TestCheckIncludeModule(c *gc.C) {
	c.Check(checkIncludeModule("juju"), jc.IsTrue)
	c.Check(checkIncludeModule("juju", "juju"), jc.IsTrue)
	c.Check(checkIncludeModule("juju", "juju.environ"), jc.IsFalse)
	c.Check(checkIncludeModule("juju.provisioner", "juju"), jc.IsTrue)
	c.Check(checkIncludeModule("juju.provisioner", "juju*"), jc.IsFalse)
	c.Check(checkIncludeModule("juju.provisioner", "juju.environ"), jc.IsFalse)
	c.Check(checkIncludeModule("unit.mysql/1", "juju", "unit"), jc.IsTrue)
}

func checkExcludeEntity(logValue string, agent ...string) bool {
	stream := newLogFileStream(&debugLogParams{
		excludeEntity: agent,
	})
	line := &logFileLine{agentTag: logValue}
	return stream.exclude(line)
}

func (s *debugLogFileIntSuite) TestCheckExcludeEntity(c *gc.C) {
	c.Check(checkExcludeEntity("machine-0"), jc.IsFalse)
	c.Check(checkExcludeEntity("machine-0", "machine-0"), jc.IsTrue)
	c.Check(checkExcludeEntity("machine-1", "machine-0"), jc.IsFalse)
	c.Check(checkExcludeEntity("machine-1", "machine-0", "machine-1"), jc.IsTrue)
	c.Check(checkExcludeEntity("machine-0-lxc-0", "machine-0"), jc.IsFalse)
	c.Check(checkExcludeEntity("machine-0-lxc-0", "machine-0*"), jc.IsTrue)
	c.Check(checkExcludeEntity("machine-0-lxc-0", "machine-0-lxc-*"), jc.IsTrue)
}

func checkExcludeModule(logValue string, module ...string) bool {
	stream := newLogFileStream(&debugLogParams{
		excludeModule: module,
	})
	line := &logFileLine{module: logValue}
	return stream.exclude(line)
}

func (s *debugLogFileIntSuite) TestCheckExcludeModule(c *gc.C) {
	c.Check(checkExcludeModule("juju"), jc.IsFalse)
	c.Check(checkExcludeModule("juju", "juju"), jc.IsTrue)
	c.Check(checkExcludeModule("juju", "juju.environ"), jc.IsFalse)
	c.Check(checkExcludeModule("juju.provisioner", "juju"), jc.IsTrue)
	c.Check(checkExcludeModule("juju.provisioner", "juju*"), jc.IsFalse)
	c.Check(checkExcludeModule("juju.provisioner", "juju.environ"), jc.IsFalse)
	c.Check(checkExcludeModule("unit.mysql/1", "juju", "unit"), jc.IsTrue)
}

func (s *debugLogFileIntSuite) TestFilterLine(c *gc.C) {
	stream := newLogFileStream(&debugLogParams{
		filterLevel:   loggo.INFO,
		includeEntity: []string{"machine-0", "unit-mysql*"},
		includeModule: []string{"juju"},
		excludeEntity: []string{"unit-mysql-2"},
		excludeModule: []string{"juju.foo"},
	})
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

func (s *debugLogFileIntSuite) TestCountedFilterLineWithLimit(c *gc.C) {
	stream := newLogFileStream(&debugLogParams{
		filterLevel: loggo.INFO,
		maxLines:    5,
	})
	line := []byte("machine-0: date time WARNING juju")
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsTrue)
	c.Check(stream.countedFilterLine(line), jc.IsFalse)
	c.Check(stream.countedFilterLine(line), jc.IsFalse)
}

type chanWriter struct {
	ch chan []byte
}

func (w *chanWriter) Write(buf []byte) (n int, err error) {
	bufcopy := append([]byte{}, buf...)
	w.ch <- bufcopy
	return len(buf), nil
}

func (s *debugLogFileIntSuite) testStreamInternal(c *gc.C, fromTheStart bool, backlog, maxLines uint, expected, errMatch string) {

	dir := c.MkDir()
	logPath := filepath.Join(dir, "logfile.txt")
	logFile, err := os.Create(logPath)
	c.Assert(err, jc.ErrorIsNil)
	defer logFile.Close()
	logFileReader, err := os.Open(logPath)
	c.Assert(err, jc.ErrorIsNil)
	defer logFileReader.Close()

	logFile.WriteString(`line 1
line 2
line 3
`)

	stream := newLogFileStream(&debugLogParams{
		fromTheStart: fromTheStart,
		backlog:      backlog,
		maxLines:     maxLines,
	})
	err = stream.positionLogFile(logFileReader)
	c.Assert(err, jc.ErrorIsNil)
	var output bytes.Buffer
	writer := &chanWriter{make(chan []byte)}
	stream.start(logFileReader, writer)
	defer stream.logTailer.Stop()

	logFile.WriteString("line 4\n")
	logFile.WriteString("line 5\n")

	timeout := time.After(testing.LongWait)
	for output.String() != expected {
		select {
		case buf := <-writer.ch:
			output.Write(buf)
		case <-timeout:
			c.Fatalf("expected data didn't arrive:\n\tobtained: %#v\n\texpected: %#v", output.String(), expected)
		}
	}

	stream.logTailer.Stop()

	err = stream.wait(nil)
	if errMatch == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, errMatch)
	}
}

func (s *debugLogFileIntSuite) TestLogStreamLoopFromTheStart(c *gc.C) {
	expected := `line 1
line 2
line 3
line 4
line 5
`
	s.testStreamInternal(c, true, 0, 0, expected, "")
}

func (s *debugLogFileIntSuite) TestLogStreamLoopFromTheStartMaxLines(c *gc.C) {
	expected := `line 1
line 2
line 3
`
	s.testStreamInternal(c, true, 0, 3, expected, "")
}

func (s *debugLogFileIntSuite) TestLogStreamLoopJustTail(c *gc.C) {
	expected := `line 4
line 5
`
	s.testStreamInternal(c, false, 0, 0, expected, "")
}

func (s *debugLogFileIntSuite) TestLogStreamLoopBackOneLimitTwo(c *gc.C) {
	expected := `line 3
line 4
`
	s.testStreamInternal(c, false, 1, 2, expected, "")
}

func (s *debugLogFileIntSuite) TestLogStreamLoopTailMaxLinesNotYetReached(c *gc.C) {
	expected := `line 4
line 5
`
	s.testStreamInternal(c, false, 0, 3, expected, "")
}

func assertStreamParams(c *gc.C, obtained, expected *logFileStream) {
	c.Check(obtained.includeEntity, jc.DeepEquals, expected.includeEntity)
	c.Check(obtained.includeModule, jc.DeepEquals, expected.includeModule)
	c.Check(obtained.excludeEntity, jc.DeepEquals, expected.excludeEntity)
	c.Check(obtained.excludeModule, jc.DeepEquals, expected.excludeModule)
	c.Check(obtained.maxLines, gc.Equals, expected.maxLines)
	c.Check(obtained.fromTheStart, gc.Equals, expected.fromTheStart)
	c.Check(obtained.filterLevel, gc.Equals, expected.filterLevel)
	c.Check(obtained.backlog, gc.Equals, expected.backlog)
}

func (s *debugLogFileIntSuite) TestNewLogStream(c *gc.C) {
	params, err := readDebugLogParams(url.Values{
		"includeEntity": []string{"machine-1*", "machine-2"},
		"includeModule": []string{"juju", "unit"},
		"excludeEntity": []string{"machine-1-lxc*"},
		"excludeModule": []string{"juju.provisioner"},
		"maxLines":      []string{"300"},
		"backlog":       []string{"100"},
		"level":         []string{"INFO"},
		// OK, just a little nonsense
		"replay": []string{"true"},
	})
	c.Assert(err, jc.ErrorIsNil)

	assertStreamParams(c, newLogFileStream(params), &logFileStream{
		debugLogParams: &debugLogParams{
			includeEntity: []string{"machine-1*", "machine-2"},
			includeModule: []string{"juju", "unit"},
			excludeEntity: []string{"machine-1-lxc*"},
			excludeModule: []string{"juju.provisioner"},
			maxLines:      300,
			backlog:       100,
			filterLevel:   loggo.INFO,
			fromTheStart:  true,
		},
	})
}

func (s *debugLogFileIntSuite) TestParamErrors(c *gc.C) {

	_, err := readDebugLogParams(url.Values{"maxLines": []string{"foo"}})
	c.Assert(err, gc.ErrorMatches, `maxLines value "foo" is not a valid unsigned number`)

	_, err = readDebugLogParams(url.Values{"backlog": []string{"foo"}})
	c.Assert(err, gc.ErrorMatches, `backlog value "foo" is not a valid unsigned number`)

	_, err = readDebugLogParams(url.Values{"replay": []string{"foo"}})
	c.Assert(err, gc.ErrorMatches, `replay value "foo" is not a valid boolean`)

	_, err = readDebugLogParams(url.Values{"level": []string{"foo"}})
	c.Assert(err, gc.ErrorMatches, `level value "foo" is not one of "TRACE", "DEBUG", "INFO", "WARNING", "ERROR"`)
}

type agentMatchTest struct {
	about    string
	line     string
	filter   string
	expected bool
}

var agentMatchTests []agentMatchTest = []agentMatchTest{
	{
		about:    "Matching with wildcard - match everything",
		line:     "machine-1: sdscsc",
		filter:   "*",
		expected: true,
	}, {
		about:    "Matching with wildcard as suffix - match machine tag...",
		line:     "machine-1: sdscsc",
		filter:   "mach*",
		expected: true,
	}, {
		about:    "Matching with wildcard as prefix - match machine tag...",
		line:     "machine-1: sdscsc",
		filter:   "*ch*",
		expected: true,
	}, {
		about:    "Matching with wildcard in the middle - match machine tag...",
		line:     "machine-1: sdscsc",
		filter:   "mach*1",
		expected: true,
	}, {
		about:    "Matching with wildcard - match machine name",
		line:     "machine-1: sdscsc",
		filter:   "1*",
		expected: true,
	}, {
		about:    "Matching exact machine name",
		line:     "machine-1: sdscsc",
		filter:   "2",
		expected: false,
	}, {
		about:    "Matching invalid filter",
		line:     "machine-1: sdscsc",
		filter:   "my-service",
		expected: false,
	}, {
		about:    "Matching exact machine tag",
		line:     "machine-1: sdscsc",
		filter:   "machine-1",
		expected: true,
	}, {
		about:    "Matching exact machine tag = not equal",
		line:     "machine-1: sdscsc",
		filter:   "machine-3",
		expected: false,
	}, {
		about:    "Matching with wildcard - match unit tag...",
		line:     "unit-ubuntu-1: sdscsc",
		filter:   "un*",
		expected: true,
	}, {
		about:    "Matching with wildcard - match unit name",
		line:     "unit-ubuntu-1: sdscsc",
		filter:   "ubuntu*",
		expected: true,
	}, {
		about:    "Matching exact unit name",
		line:     "unit-ubuntu-1: sdscsc",
		filter:   "ubuntu/2",
		expected: false,
	}, {
		about:    "Matching exact unit tag",
		line:     "unit-ubuntu-1: sdscsc",
		filter:   "unit-ubuntu-1",
		expected: true,
	}, {
		about:    "Matching exact unit tag = not equal",
		line:     "unit-ubuntu-2: sdscsc",
		filter:   "unit-ubuntu-1",
		expected: false,
	},
}

// TestAgentMatchesFilter tests that line agent matches desired filter as expected
func (s *debugLogFileIntSuite) TestAgentMatchesFilter(c *gc.C) {
	for i, test := range agentMatchTests {
		c.Logf("test %d: %v\n", i, test.about)
		matched := AgentMatchesFilter(ParseLogLine(test.line), test.filter)
		c.Assert(matched, gc.Equals, test.expected)
	}
}

// TestAgentLineFragmentParsing tests that agent tag and name are parsed correctly from log line
func (s *debugLogFileIntSuite) TestAgentLineFragmentParsing(c *gc.C) {
	checkAgentParsing(c, "Drop trailing colon", "machine-1: sdscsc", "machine-1", "1")
	checkAgentParsing(c, "Drop unit specific [", "unit-ubuntu-1[blah777787]: scscdcdc", "unit-ubuntu-1", "ubuntu/1")
	checkAgentParsing(c, "No colon in log line - invalid", "unit-ubuntu-1 scscdcdc", "", "")
}

func checkAgentParsing(c *gc.C, about, line, tag, name string) {
	c.Logf("test %q\n", about)
	logLine := ParseLogLine(line)
	c.Assert(logLine.LogLineAgentTag(), gc.Equals, tag)
	c.Assert(logLine.LogLineAgentName(), gc.Equals, name)
}
