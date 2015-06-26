// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
)

type debugLogFileSuite struct {
	debugLogBaseSuite
	logFile *os.File
	last    int
}

var _ = gc.Suite(&debugLogFileSuite{})

func (s *debugLogFileSuite) TestNoLogfile(c *gc.C) {
	reader := s.openWebsocket(c, nil)
	assertJSONError(c, reader, "cannot open log file: .*: "+utils.NoSuchFileErrRegexp)
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogFileSuite) assertLogReader(c *gc.C, reader *bufio.Reader) {
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, logLineCount)
	c.Assert(linesRead, jc.DeepEquals, logLines)
}

func (s *debugLogFileSuite) TestServesLog(c *gc.C) {
	s.ensureLogFile(c)
	reader := s.openWebsocket(c, nil)
	s.assertLogReader(c, reader)
}

func (s *debugLogFileSuite) TestReadFromTopLevelPath(c *gc.C) {
	// Backwards compatibility check, that we can read the log file at
	// https://host:port/log
	s.ensureLogFile(c)
	reader := s.openWebsocketCustomPath(c, "/log")
	s.assertLogReader(c, reader)
}

func (s *debugLogFileSuite) TestReadFromEnvUUIDPath(c *gc.C) {
	// Check that we can read the log at https://host:port/ENVUUID/log
	environ, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	s.ensureLogFile(c)
	reader := s.openWebsocketCustomPath(c, fmt.Sprintf("/environment/%s/log", environ.UUID()))
	s.assertLogReader(c, reader)
}

func (s *debugLogFileSuite) TestReadRejectsWrongEnvUUIDPath(c *gc.C) {
	// Check that we cannot pull logs from https://host:port/BADENVUUID/log
	s.ensureLogFile(c)
	reader := s.openWebsocketCustomPath(c, "/environment/dead-beef-123456/log")
	assertJSONError(c, reader, `unknown environment: "dead-beef-123456"`)
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogFileSuite) TestReadsFromEnd(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, nil)
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, logLineCount-10)
	c.Assert(linesRead, jc.DeepEquals, logLines[10:])
}

func (s *debugLogFileSuite) TestReplayFromStart(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, url.Values{"replay": {"true"}})
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, logLineCount)
	c.Assert(linesRead, jc.DeepEquals, logLines)
}

func (s *debugLogFileSuite) TestBacklog(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, url.Values{"backlog": {"5"}})
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, logLineCount-5)
	c.Assert(linesRead, jc.DeepEquals, logLines[5:])
}

func (s *debugLogFileSuite) TestMaxLines(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, url.Values{"maxLines": {"10"}})
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, 10)
	c.Assert(linesRead, jc.DeepEquals, logLines[10:20])
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogFileSuite) TestBacklogWithMaxLines(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, url.Values{"backlog": {"5"}, "maxLines": {"10"}})
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, 10)
	c.Assert(linesRead, jc.DeepEquals, logLines[5:15])
	s.assertWebsocketClosed(c, reader)
}

type filterTest struct {
	about    string
	filter   url.Values
	filtered []string
}

var filterTests []filterTest = []filterTest{
	{
		about: "Filter from original test",
		filter: url.Values{
			"includeEntity": {"machine-0", "unit-ubuntu-0"},
			"includeModule": {"juju.cmd"},
			"excludeModule": {"juju.cmd.jujud"},
		},
		filtered: []string{logLines[0], logLines[40]},
	}, {
		about: "Filter from original test inverted",
		filter: url.Values{
			"excludeEntity": {"machine-1"},
		},
		filtered: []string{logLines[0], logLines[1]},
	}, {
		about: "Include Entity Filter with only wildcard",
		filter: url.Values{
			"includeEntity": {"*"},
		},
		filtered: []string{logLines[0], logLines[1]},
	}, {
		about: "Exclude Entity Filter with only wildcard",
		filter: url.Values{
			"excludeEntity": {"*"}, // exclude everything :-)
		},
		filtered: []string{},
	}, {
		about: "Include Entity Filter with 1 wildcard",
		filter: url.Values{
			"includeEntity": {"unit-*"},
		},
		filtered: []string{logLines[40], logLines[41]},
	}, {
		about: "Exclude Entity Filter with 1 wildcard",
		filter: url.Values{
			"excludeEntity": {"machine-*"},
		},
		filtered: []string{logLines[40], logLines[41]},
	}, {
		about: "Include Entity Filter using machine tag",
		filter: url.Values{
			"includeEntity": {"machine-1"},
		},
		filtered: []string{logLines[27], logLines[28]},
	}, {
		about: "Include Entity Filter using machine name",
		filter: url.Values{
			"includeEntity": {"1"},
		},
		filtered: []string{logLines[27], logLines[28]},
	}, {
		about: "Include Entity Filter using unit tag",
		filter: url.Values{
			"includeEntity": {"unit-ubuntu-0"},
		},
		filtered: []string{logLines[40], logLines[41]},
	}, {
		about: "Include Entity Filter using unit name",
		filter: url.Values{
			"includeEntity": {"ubuntu/0"},
		},
		filtered: []string{logLines[40], logLines[41]},
	}, {
		about: "Include Entity Filter using combination of machine tag and unit name",
		filter: url.Values{
			"includeEntity": {"machine-1", "ubuntu/0"},
			"includeModule": {"juju.agent"},
		},
		filtered: []string{logLines[29], logLines[34], logLines[41]},
	}, {
		about: "Exclude Entity Filter using machine tag",
		filter: url.Values{
			"excludeEntity": {"machine-0"},
		},
		filtered: []string{logLines[27], logLines[28]},
	}, {
		about: "Exclude Entity Filter using machine name",
		filter: url.Values{
			"excludeEntity": {"0"},
		},
		filtered: []string{logLines[27], logLines[28]},
	}, {
		about: "Exclude Entity Filter using unit tag",
		filter: url.Values{
			"excludeEntity": {"machine-0", "machine-1", "unit-ubuntu-0"},
		},
		filtered: []string{logLines[54], logLines[55]},
	}, {
		about: "Exclude Entity Filter using unit name",
		filter: url.Values{
			"excludeEntity": {"machine-0", "machine-1", "ubuntu/0"},
		},
		filtered: []string{logLines[54], logLines[55]},
	}, {
		about: "Exclude Entity Filter using combination of machine tag and unit name",
		filter: url.Values{
			"excludeEntity": {"0", "1", "ubuntu/0"},
		},
		filtered: []string{logLines[54], logLines[55]},
	},
}

// TestFilter tests that filters are processed correctly given specific debug-log configuration.
func (s *debugLogFileSuite) TestFilter(c *gc.C) {
	for i, test := range filterTests {
		c.Logf("test %d: %v\n", i, test.about)

		// ensures log file
		path := filepath.Join(s.LogDir, "all-machines.log")
		var err error
		s.logFile, err = os.Create(path)
		c.Assert(err, jc.ErrorIsNil)

		// opens web socket
		conn := s.dialWebsocket(c, test.filter)
		reader := bufio.NewReader(conn)

		s.assertLogFollowing(c, reader)
		s.writeLogLines(c, logLineCount)
		/*
			This will filter and return as many lines as filtered wanted to examine.
			 So, if specified filter can potentially return 40 lines from sample log but filtered only wanted 2,
			 then the first 2 lines that match the filter will be returned here.
		*/
		linesRead := s.readLogLines(c, reader, len(test.filtered))
		// compare retrieved lines with expected
		c.Assert(linesRead, jc.DeepEquals, test.filtered)

		// release resources
		conn.Close()
		s.logFile.Close()
		s.logFile = nil
		s.last = 0
	}
}

// readLogLines filters and returns as many lines as filtered wanted to examine.
// So, if specified filter can potentially return 40 lines from sample log but filtered only wanted 2,
// then the first 2 lines that match the filter will be returned here.
func (s *debugLogFileSuite) readLogLines(c *gc.C, reader *bufio.Reader, count int) (linesRead []string) {
	for len(linesRead) < count {
		line, err := reader.ReadString('\n')
		c.Assert(err, jc.ErrorIsNil)
		// Trim off the trailing \n
		linesRead = append(linesRead, line[:len(line)-1])
	}
	return linesRead
}

func (s *debugLogFileSuite) ensureLogFile(c *gc.C) {
	if s.logFile != nil {
		return
	}
	path := filepath.Join(s.LogDir, "all-machines.log")
	var err error
	s.logFile, err = os.Create(path)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		s.logFile.Close()
		s.logFile = nil
		s.last = 0
	})
}

func (s *debugLogFileSuite) writeLogLines(c *gc.C, count int) {
	s.ensureLogFile(c)
	for i := 0; i < count && s.last < logLineCount; i++ {
		s.logFile.WriteString(logLines[s.last] + "\n")
		s.last++
	}
}

func (s *debugLogFileSuite) assertLogFollowing(c *gc.C, reader *bufio.Reader) {
	errResult := readJSONErrorLine(c, reader)
	c.Assert(errResult.Error, gc.IsNil)
}

var (
	logLines = strings.Split(`
machine-0: 2014-03-24 22:34:25 INFO juju.cmd supercommand.go:297 running juju-1.17.7.1-trusty-amd64 [gc]
machine-0: 2014-03-24 22:34:25 INFO juju.cmd.jujud machine.go:127 machine agent machine-0 start (1.17.7.1-trusty-amd64 [gc])
machine-0: 2014-03-24 22:34:25 DEBUG juju.agent agent.go:384 read agent config, format "1.18"
machine-0: 2014-03-24 22:34:25 INFO juju.cmd.jujud machine.go:155 Starting StateWorker for machine-0
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "state"
machine-0: 2014-03-24 22:34:25 INFO juju.state open.go:80 opening state; mongo addresses: ["localhost:37017"]; entity "machine-0"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "api"
machine-0: 2014-03-24 22:34:25 INFO juju apiclient.go:114 api: dialing "wss://localhost:17070/"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "termination"
machine-0: 2014-03-24 22:34:25 ERROR juju apiclient.go:119 api: websocket.Dial wss://localhost:17070/: dial tcp 127.0.0.1:17070: connection refused
machine-0: 2014-03-24 22:34:25 ERROR juju runner.go:220 worker: exited "api": websocket.Dial wss://localhost:17070/: dial tcp 127.0.0.1:17070: connection refused
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:254 worker: restarting "api" in 3s
machine-0: 2014-03-24 22:34:25 INFO juju.state open.go:118 connection established
machine-0: 2014-03-24 22:34:25 DEBUG juju.utils gomaxprocs.go:24 setting GOMAXPROCS to 8
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "local-storage"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "instancepoller"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "apiserver"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "resumer"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "cleaner"
machine-0: 2014-03-24 22:34:25 INFO juju.apiserver apiserver.go:43 listening on "[::]:17070"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "minunitsworker"
machine-0: 2014-03-24 22:34:28 INFO juju runner.go:262 worker: start "api"
machine-0: 2014-03-24 22:34:28 INFO juju apiclient.go:114 api: dialing "wss://localhost:17070/"
machine-0: 2014-03-24 22:34:28 INFO juju.apiserver apiserver.go:131 [1] API connection from 127.0.0.1:36491
machine-0: 2014-03-24 22:34:28 INFO juju apiclient.go:124 api: connection established
machine-0: 2014-03-24 22:34:28 DEBUG juju.apiserver apiserver.go:120 <- [1] <unknown> {"RequestId":1,"Type":"Admin","Request":"Login","Params":{"AuthTag":"machine-0","Password":"ARbW7iCV4LuMugFEG+Y4e0yr","Nonce":"user-admin:bootstrap"}}
machine-0: 2014-03-24 22:34:28 DEBUG juju.apiserver apiserver.go:127 -> [1] machine-0 10.305679ms {"RequestId":1,"Response":{}} Admin[""].Login
machine-1: 2014-03-24 22:36:28 INFO juju.cmd supercommand.go:297 running juju-1.17.7.1-precise-amd64 [gc]
machine-1: 2014-03-24 22:36:28 INFO juju.cmd.jujud machine.go:127 machine agent machine-1 start (1.17.7.1-precise-amd64 [gc])
machine-1: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:384 read agent config, format "1.18"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "api"
machine-1: 2014-03-24 22:36:28 INFO juju apiclient.go:114 api: dialing "wss://10.0.3.1:17070/"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "termination"
machine-1: 2014-03-24 22:36:28 INFO juju apiclient.go:124 api: connection established
machine-1: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:523 writing configuration file
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "upgrader"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "upgrade-steps"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "machiner"
machine-1: 2014-03-24 22:36:28 INFO juju.cmd.jujud machine.go:458 upgrade to 1.17.7.1-precise-amd64 already completed.
machine-1: 2014-03-24 22:36:28 INFO juju.cmd.jujud machine.go:445 upgrade to 1.17.7.1-precise-amd64 completed.
unit-ubuntu-0[32423]: 2014-03-24 22:36:28 INFO juju.cmd supercommand.go:297 running juju-1.17.7.1-precise-amd64 [gc]
unit-ubuntu-0[34543]: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:384 read agent config, format "1.18"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju.jujud unit.go:76 unit agent unit-ubuntu-0 start (1.17.7.1-precise-amd64 [gc])
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "api"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju apiclient.go:114 api: dialing "wss://10.0.3.1:17070/"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju apiclient.go:124 api: connection established
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:523 writing configuration file
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "upgrader"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "logger"
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.worker.logger logger.go:35 initial log config: "<root>=DEBUG"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "uniter"
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.worker.logger logger.go:60 logger setup
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "rsyslog"
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.worker.rsyslog worker.go:76 starting rsyslog worker mode 1 for "unit-ubuntu-0" "tim-local"
unit-ubuntu-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "logger"
unit-ubuntu-1: 2014-03-24 22:36:28 DEBUG juju.worker.logger logger.go:35 initial log config: "<root>=DEBUG"
unit-ubuntu-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "uniter"
unit-ubuntu-1: 2014-03-24 22:36:28 DEBUG juju.worker.logger logger.go:60 logger setup
unit-ubuntu-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "rsyslog"
unit-ubuntu-1: 2014-03-24 22:36:28 DEBUG juju.worker.rsyslog worker.go:76 starting rsyslog worker mode 1 for "unit-ubuntu-0" "tim-local"
`[1:], "\n")
	logLineCount = len(logLines)
)
