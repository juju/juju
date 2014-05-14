// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"code.google.com/p/go.net/websocket"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type debugLogSuite struct {
	authHttpSuite
	logFile *os.File
	last    int
}

var _ = gc.Suite(&debugLogSuite{})

func (s *debugLogSuite) TestWithHTTP(c *gc.C) {
	uri := s.logURL(c, "http", nil).String()
	_, err := s.sendRequest(c, "", "", "GET", uri, "", nil)
	c.Assert(err, gc.ErrorMatches, `.*malformed HTTP response.*`)
}

func (s *debugLogSuite) TestWithHTTPS(c *gc.C) {
	uri := s.logURL(c, "https", nil).String()
	response, err := s.sendRequest(c, "", "", "GET", uri, "", nil)
	c.Assert(err, gc.IsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusBadRequest)
}

func (s *debugLogSuite) TestNoAuth(c *gc.C) {
	conn, err := s.dialWebsocketInternal(c, nil, nil)
	c.Assert(err, gc.IsNil)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	s.assertErrorResponse(c, reader, "auth failed: invalid request format")
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogSuite) TestNoLogfile(c *gc.C) {
	reader := s.openWebsocket(c, nil)
	s.assertErrorResponse(c, reader, "cannot open log file: .*: no such file or directory")
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogSuite) TestBadParams(c *gc.C) {
	reader := s.openWebsocket(c, url.Values{"maxLines": {"foo"}})
	s.assertErrorResponse(c, reader, `maxLines value "foo" is not a valid unsigned number`)
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogSuite) TestServesLog(c *gc.C) {
	s.ensureLogFile(c)
	reader := s.openWebsocket(c, nil)
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, logLineCount)
	c.Assert(linesRead, jc.DeepEquals, logLines)
}

func (s *debugLogSuite) TestReadsFromEnd(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, nil)
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, logLineCount-10)
	c.Assert(linesRead, jc.DeepEquals, logLines[10:])
}

func (s *debugLogSuite) TestReplayFromStart(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, url.Values{"replay": {"true"}})
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, logLineCount)
	c.Assert(linesRead, jc.DeepEquals, logLines)
}

func (s *debugLogSuite) TestBacklog(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, url.Values{"backlog": {"5"}})
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, logLineCount-5)
	c.Assert(linesRead, jc.DeepEquals, logLines[5:])
}

func (s *debugLogSuite) TestMaxLines(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, url.Values{"maxLines": {"10"}})
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, 10)
	c.Assert(linesRead, jc.DeepEquals, logLines[10:20])
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogSuite) TestBacklogWithMaxLines(c *gc.C) {
	s.writeLogLines(c, 10)

	reader := s.openWebsocket(c, url.Values{"backlog": {"5"}, "maxLines": {"10"}})
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	linesRead := s.readLogLines(c, reader, 10)
	c.Assert(linesRead, jc.DeepEquals, logLines[5:15])
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogSuite) TestFilter(c *gc.C) {
	s.ensureLogFile(c)

	reader := s.openWebsocket(c, url.Values{
		"includeEntity": {"machine-0", "unit-ubuntu-0"},
		"includeModule": {"juju.cmd"},
		"excludeModule": {"juju.cmd.jujud"},
	})
	s.assertLogFollowing(c, reader)
	s.writeLogLines(c, logLineCount)

	expected := []string{logLines[0], logLines[40]}
	linesRead := s.readLogLines(c, reader, len(expected))
	c.Assert(linesRead, jc.DeepEquals, expected)
}

func (s *debugLogSuite) readLogLines(c *gc.C, reader *bufio.Reader, count int) (linesRead []string) {
	for len(linesRead) < count {
		line, err := reader.ReadString('\n')
		c.Assert(err, gc.IsNil)
		// Trim off the trailing \n
		linesRead = append(linesRead, line[:len(line)-1])
	}
	return linesRead
}

func (s *debugLogSuite) openWebsocket(c *gc.C, values url.Values) *bufio.Reader {
	conn, err := s.dialWebsocket(c, values)
	c.Assert(err, gc.IsNil)
	s.AddCleanup(func(_ *gc.C) { conn.Close() })
	return bufio.NewReader(conn)
}

func (s *debugLogSuite) ensureLogFile(c *gc.C) {
	if s.logFile != nil {
		return
	}
	path := filepath.Join(s.LogDir, "all-machines.log")
	var err error
	s.logFile, err = os.Create(path)
	c.Assert(err, gc.IsNil)
	s.AddCleanup(func(c *gc.C) {
		s.logFile.Close()
		s.logFile = nil
		s.last = 0
	})
}

func (s *debugLogSuite) writeLogLines(c *gc.C, count int) {
	s.ensureLogFile(c)
	for i := 0; i < count && s.last < logLineCount; i++ {
		s.logFile.WriteString(logLines[s.last] + "\n")
		s.last++
	}
}

func (s *debugLogSuite) dialWebsocketInternal(c *gc.C, queryParams url.Values, header http.Header) (*websocket.Conn, error) {
	server := s.logURL(c, "wss", queryParams).String()
	c.Logf("dialing %v", server)
	config, err := websocket.NewConfig(server, "http://localhost/")
	c.Assert(err, gc.IsNil)
	config.Header = header
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(testing.CACert)), jc.IsTrue)
	config.TlsConfig = &tls.Config{RootCAs: caCerts, ServerName: "anything"}
	return websocket.DialConfig(config)
}

func (s *debugLogSuite) dialWebsocket(c *gc.C, queryParams url.Values) (*websocket.Conn, error) {
	header := utils.BasicAuthHeader(s.userTag, s.password)
	return s.dialWebsocketInternal(c, queryParams, header)
}

func (s *debugLogSuite) logURL(c *gc.C, scheme string, queryParams url.Values) *url.URL {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	query := ""
	if queryParams != nil {
		query = queryParams.Encode()
	}
	return &url.URL{
		Scheme:   scheme,
		Host:     info.Addrs[0],
		Path:     "/log",
		RawQuery: query,
	}
}

func (s *debugLogSuite) assertWebsocketClosed(c *gc.C, reader *bufio.Reader) {
	_, err := reader.ReadByte()
	c.Assert(err, gc.Equals, io.EOF)
}

func (s *debugLogSuite) assertLogFollowing(c *gc.C, reader *bufio.Reader) {
	errResult := s.getErrorResult(c, reader)
	c.Assert(errResult.Error, gc.IsNil)
}

func (s *debugLogSuite) assertErrorResponse(c *gc.C, reader *bufio.Reader, expected string) {
	errResult := s.getErrorResult(c, reader)
	c.Assert(errResult.Error, gc.NotNil)
	c.Assert(errResult.Error.Message, gc.Matches, expected)
}

func (s *debugLogSuite) getErrorResult(c *gc.C, reader *bufio.Reader) params.ErrorResult {
	line, err := reader.ReadSlice('\n')
	c.Assert(err, gc.IsNil)
	var errResult params.ErrorResult
	err = json.Unmarshal(line, &errResult)
	c.Assert(err, gc.IsNil)
	return errResult
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
machine-0: 2014-03-24 22:34:25 INFO juju apiclient.go:114 state/api: dialing "wss://localhost:17070/"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "termination"
machine-0: 2014-03-24 22:34:25 ERROR juju apiclient.go:119 state/api: websocket.Dial wss://localhost:17070/: dial tcp 127.0.0.1:17070: connection refused
machine-0: 2014-03-24 22:34:25 ERROR juju runner.go:220 worker: exited "api": websocket.Dial wss://localhost:17070/: dial tcp 127.0.0.1:17070: connection refused
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:254 worker: restarting "api" in 3s
machine-0: 2014-03-24 22:34:25 INFO juju.state open.go:118 connection established
machine-0: 2014-03-24 22:34:25 DEBUG juju.utils gomaxprocs.go:24 setting GOMAXPROCS to 8
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "local-storage"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "instancepoller"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "apiserver"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "resumer"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "cleaner"
machine-0: 2014-03-24 22:34:25 INFO juju.state.apiserver apiserver.go:43 listening on "[::]:17070"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "minunitsworker"
machine-0: 2014-03-24 22:34:28 INFO juju runner.go:262 worker: start "api"
machine-0: 2014-03-24 22:34:28 INFO juju apiclient.go:114 state/api: dialing "wss://localhost:17070/"
machine-0: 2014-03-24 22:34:28 INFO juju.state.apiserver apiserver.go:131 [1] API connection from 127.0.0.1:36491
machine-0: 2014-03-24 22:34:28 INFO juju apiclient.go:124 state/api: connection established
machine-0: 2014-03-24 22:34:28 DEBUG juju.state.apiserver apiserver.go:120 <- [1] <unknown> {"RequestId":1,"Type":"Admin","Request":"Login","Params":{"AuthTag":"machine-0","Password":"ARbW7iCV4LuMugFEG+Y4e0yr","Nonce":"user-admin:bootstrap"}}
machine-0: 2014-03-24 22:34:28 DEBUG juju.state.apiserver apiserver.go:127 -> [1] machine-0 10.305679ms {"RequestId":1,"Response":{}} Admin[""].Login
machine-1: 2014-03-24 22:36:28 INFO juju.cmd supercommand.go:297 running juju-1.17.7.1-precise-amd64 [gc]
machine-1: 2014-03-24 22:36:28 INFO juju.cmd.jujud machine.go:127 machine agent machine-1 start (1.17.7.1-precise-amd64 [gc])
machine-1: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:384 read agent config, format "1.18"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "api"
machine-1: 2014-03-24 22:36:28 INFO juju apiclient.go:114 state/api: dialing "wss://10.0.3.1:17070/"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "termination"
machine-1: 2014-03-24 22:36:28 INFO juju apiclient.go:124 state/api: connection established
machine-1: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:523 writing configuration file
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "upgrader"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "upgrade-steps"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "machiner"
machine-1: 2014-03-24 22:36:28 INFO juju.cmd.jujud machine.go:458 upgrade to 1.17.7.1-precise-amd64 already completed.
machine-1: 2014-03-24 22:36:28 INFO juju.cmd.jujud machine.go:445 upgrade to 1.17.7.1-precise-amd64 completed.
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju.cmd supercommand.go:297 running juju-1.17.7.1-precise-amd64 [gc]
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:384 read agent config, format "1.18"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju.jujud unit.go:76 unit agent unit-ubuntu-0 start (1.17.7.1-precise-amd64 [gc])
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "api"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju apiclient.go:114 state/api: dialing "wss://10.0.3.1:17070/"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju apiclient.go:124 state/api: connection established
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:523 writing configuration file
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "upgrader"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "logger"
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.worker.logger logger.go:35 initial log config: "<root>=DEBUG"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "uniter"
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.worker.logger logger.go:60 logger setup
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "rsyslog"
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.worker.rsyslog worker.go:76 starting rsyslog worker mode 1 for "unit-ubuntu-0" "tim-local"
`[1:], "\n")
	logLineCount = len(logLines)
)
