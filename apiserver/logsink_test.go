// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/websocket/websockettest"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

type logsinkSuite struct {
	apiserverBaseSuite
	machineTag names.Tag
	password   string
	nonce      string
	logs       loggo.TestWriter
	url        string
}

var _ = gc.Suite(&logsinkSuite{})

func (s *logsinkSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)
	s.nonce = "nonce"
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: s.nonce,
	})
	s.machineTag = m.Tag()
	s.password = password

	url := s.URL(
		"/model/"+s.State.ModelUUID()+"/logsink",
		url.Values{"jujuclientversion": {version.Current.String()}},
	)
	url.Scheme = "wss"
	s.url = url.String()

	s.logs.Clear()
	writer := loggo.NewMinimumLevelWriter(&s.logs, loggo.INFO)
	c.Assert(loggo.RegisterWriter("logsink-tests", writer), jc.ErrorIsNil)
}

func (s *logsinkSuite) TestNoAuth(c *gc.C) {
	s.checkAuthFails(c, nil, http.StatusUnauthorized, "authentication failed: no credentials provided")
}

func (s *logsinkSuite) TestRejectsUserLogins(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "sekrit"})
	header := utils.BasicAuthHeader(user.Tag().String(), "sekrit")
	s.checkAuthFails(c, header, http.StatusForbidden, "authorization failed: tag kind user not valid")
}

func (s *logsinkSuite) checkAuthFails(c *gc.C, header http.Header, code int, message string) {
	_, resp, err := dialWebsocketFromURL(c, s.url, header)
	c.Assert(err, gc.Equals, websocket.ErrBadHandshake)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, code)
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(body), gc.Equals, message+"\n")
}

func (s *logsinkSuite) TestLogging(c *gc.C) {
	conn := s.dialWebsocket(c)
	defer conn.Close()

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	err := conn.WriteJSON(&params.LogRecord{
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	})
	c.Assert(err, jc.ErrorIsNil)

	t1 := time.Date(2015, time.June, 1, 23, 2, 2, 0, time.UTC)
	err = conn.WriteJSON(&params.LogRecord{
		Time:     t1,
		Module:   "else.where",
		Location: "bar.go:99",
		Level:    loggo.ERROR.String(),
		Message:  "oh noes",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the log documents to be written to the DB.
	logsColl := s.State.MongoSession().DB("logs").C("logs." + s.State.ModelUUID())
	var docs []bson.M
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		err := logsColl.Find(nil).Sort("t").All(&docs)
		c.Assert(err, jc.ErrorIsNil)
		if len(docs) == 2 {
			break
		}
		if len(docs) >= 2 {
			c.Fatalf("saw more log documents than expected")
		}
		if !a.HasNext() {
			c.Fatalf("timed out waiting for log writes")
		}
	}

	// Check the recorded logs are correct.
	modelUUID := s.State.ModelUUID()
	c.Assert(docs[0]["t"], gc.Equals, t0.UnixNano())
	c.Assert(docs[0]["n"], gc.Equals, s.machineTag.String())
	c.Assert(docs[0]["m"], gc.Equals, "some.where")
	c.Assert(docs[0]["l"], gc.Equals, "foo.go:42")
	c.Assert(docs[0]["v"], gc.Equals, int(loggo.INFO))
	c.Assert(docs[0]["x"], gc.Equals, "all is well")

	c.Assert(docs[1]["t"], gc.Equals, t1.UnixNano())
	c.Assert(docs[1]["n"], gc.Equals, s.machineTag.String())
	c.Assert(docs[1]["m"], gc.Equals, "else.where")
	c.Assert(docs[1]["l"], gc.Equals, "bar.go:99")
	c.Assert(docs[1]["v"], gc.Equals, int(loggo.ERROR))
	c.Assert(docs[1]["x"], gc.Equals, "oh noes")

	// Close connection.
	err = conn.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that no error is logged when the connection is closed
	// normally.
	shortAttempt := &utils.AttemptStrategy{
		Total: coretesting.ShortWait,
		Delay: 2 * time.Millisecond,
	}
	for a := shortAttempt.Start(); a.Next(); {
		for _, log := range s.logs.Log() {
			c.Assert(log.Level, jc.LessThan, loggo.ERROR, gc.Commentf("log: %#v", log))
		}
	}

	// Check that the logsink log file was populated as expected
	logPath := filepath.Join(s.config.LogDir, "logsink.log")
	logContents, err := ioutil.ReadFile(logPath)
	c.Assert(err, jc.ErrorIsNil)
	line0 := modelUUID + ": machine-0 2015-06-01 23:02:01 INFO some.where foo.go:42 all is well\n"
	line1 := modelUUID + ": machine-0 2015-06-01 23:02:02 ERROR else.where bar.go:99 oh noes\n"
	c.Assert(string(logContents), gc.Equals, line0+line1)

	// Check the file mode is as expected. This doesn't work on
	// Windows (but this code is very unlikely to run on Windows so
	// it's ok).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(logPath)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info.Mode(), gc.Equals, os.FileMode(0640))
	}
}

func (s *logsinkSuite) TestReceiveErrorBreaksConn(c *gc.C) {
	conn := s.dialWebsocket(c)
	defer conn.Close()

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	// The logsink handler expects JSON messages. Send some
	// junk to verify that the server closes the connection.
	err := conn.WriteMessage(websocket.TextMessage, []byte("junk!"))
	c.Assert(err, jc.ErrorIsNil)

	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *logsinkSuite) TestNewServerValidatesLogSinkConfig(c *gc.C) {
	cfg := s.config
	// Make a fake-ish state pool.
	cfg.StatePool = &state.StatePool{}
	cfg.Mux = apiserverhttp.NewMux()
	cfg.Authenticator = &mockAuthenticator{}

	cfg.LogSinkConfig = &apiserver.LogSinkConfig{}

	_, err := apiserver.NewServer(cfg)
	c.Assert(err, gc.ErrorMatches, "validating logsink configuration: DBLoggerBufferSize 0 <= 0 or > 1000 not valid")

	cfg.LogSinkConfig.DBLoggerBufferSize = 1001
	_, err = apiserver.NewServer(cfg)
	c.Assert(err, gc.ErrorMatches, "validating logsink configuration: DBLoggerBufferSize 1001 <= 0 or > 1000 not valid")

	cfg.LogSinkConfig.DBLoggerBufferSize = 1
	_, err = apiserver.NewServer(cfg)
	c.Assert(err, gc.ErrorMatches, "validating logsink configuration: DBLoggerFlushInterval 0s <= 0 or > 10 seconds not valid")

	cfg.LogSinkConfig.DBLoggerFlushInterval = 30 * time.Second
	_, err = apiserver.NewServer(cfg)
	c.Assert(err, gc.ErrorMatches, "validating logsink configuration: DBLoggerFlushInterval 30s <= 0 or > 10 seconds not valid")

	cfg.LogSinkConfig.DBLoggerFlushInterval = 10 * time.Second
	_, err = apiserver.NewServer(cfg)
	c.Assert(err, gc.ErrorMatches, "validating logsink configuration: RateLimitBurst 0 <= 0 not valid")

	cfg.LogSinkConfig.RateLimitBurst = 1000
	_, err = apiserver.NewServer(cfg)
	c.Assert(err, gc.ErrorMatches, "validating logsink configuration: RateLimitRefill 0s <= 0 not valid")
}

func (s *logsinkSuite) dialWebsocket(c *gc.C) *websocket.Conn {
	conn, _, err := dialWebsocketFromURL(c, s.url, s.makeAuthHeader())
	c.Assert(err, jc.ErrorIsNil)
	return conn
}

func (s *logsinkSuite) makeAuthHeader() http.Header {
	header := utils.BasicAuthHeader(s.machineTag.String(), s.password)
	header.Add(params.MachineNonceHeader, s.nonce)
	return header
}
