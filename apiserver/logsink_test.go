// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bufio"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/apiserver"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// logsinkBaseSuite has functionality that's shared between the the 2 logsink related suites
type logsinkBaseSuite struct {
	authHttpSuite
}

func (s *logsinkBaseSuite) logsinkURL(c *gc.C, scheme string) *url.URL {
	return s.makeURL(c, scheme, "/environment/"+s.State.EnvironUUID()+"/logsink", nil)
}

type logsinkSuite struct {
	logsinkBaseSuite
	machineTag names.Tag
	password   string
	nonce      string
	logs       loggo.TestWriter
}

var _ = gc.Suite(&logsinkSuite{})

func (s *logsinkSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags("db-log")
	s.logsinkBaseSuite.SetUpTest(c)
	s.nonce = "nonce"
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: s.nonce,
	})
	s.machineTag = m.Tag()
	s.password = password

	s.logs.Clear()
	c.Assert(loggo.RegisterWriter("logsink-tests", &s.logs, loggo.INFO), jc.ErrorIsNil)
}

func (s *logsinkSuite) TestRejectsBadEnvironUUID(c *gc.C) {
	reader := s.openWebsocketCustomPath(c, "/environment/does-not-exist/logsink")
	assertJSONError(c, reader, `unknown environment: "does-not-exist"`)
	s.assertWebsocketClosed(c, reader)
}

func (s *logsinkSuite) TestNoAuth(c *gc.C) {
	s.checkAuthFails(c, nil, "invalid request format")
}

func (s *logsinkSuite) TestRejectsUserLogins(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "sekrit"})
	header := utils.BasicAuthHeader(user.Tag().String(), "sekrit")
	s.checkAuthFailsWithEntityError(c, header)
}

func (s *logsinkSuite) TestRejectsBadPassword(c *gc.C) {
	header := utils.BasicAuthHeader(s.machineTag.String(), "wrong")
	header.Add("X-Juju-Nonce", s.nonce)
	s.checkAuthFailsWithEntityError(c, header)
}

func (s *logsinkSuite) TestRejectsIncorrectNonce(c *gc.C) {
	header := utils.BasicAuthHeader(s.machineTag.String(), s.password)
	header.Add("X-Juju-Nonce", "wrong")
	s.checkAuthFails(c, header, "machine 0 not provisioned")
}

func (s *logsinkSuite) checkAuthFailsWithEntityError(c *gc.C, header http.Header) {
	s.checkAuthFails(c, header, "invalid entity name or password")
}

func (s *logsinkSuite) checkAuthFails(c *gc.C, header http.Header, message string) {
	conn := s.dialWebsocketInternal(c, header)
	defer conn.Close()
	reader := bufio.NewReader(conn)
	assertJSONError(c, reader, "auth failed: "+message)
	s.assertWebsocketClosed(c, reader)
}

func (s *logsinkSuite) TestLogging(c *gc.C) {
	conn := s.dialWebsocket(c)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// Read back the nil error, indicating that all is well.
	errResult := readJSONErrorLine(c, reader)
	c.Assert(errResult.Error, gc.IsNil)

	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	err := websocket.JSON.Send(conn, &apiserver.LogMessage{
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO,
		Message:  "all is well",
	})
	c.Assert(err, jc.ErrorIsNil)

	t1 := time.Date(2015, time.June, 1, 23, 2, 2, 0, time.UTC)
	err = websocket.JSON.Send(conn, &apiserver.LogMessage{
		Time:     t1,
		Module:   "else.where",
		Location: "bar.go:99",
		Level:    loggo.ERROR,
		Message:  "oh noes",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the log documents to be written to the DB.
	logsColl := s.State.MongoSession().DB("logs").C("logs")
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
	envUUID := s.State.EnvironUUID()
	c.Assert(docs[0]["t"].(time.Time).Sub(t0), gc.Equals, time.Duration(0))
	c.Assert(docs[0]["e"], gc.Equals, envUUID)
	c.Assert(docs[0]["n"], gc.Equals, s.machineTag.String())
	c.Assert(docs[0]["m"], gc.Equals, "some.where")
	c.Assert(docs[0]["l"], gc.Equals, "foo.go:42")
	c.Assert(docs[0]["v"], gc.Equals, int(loggo.INFO))
	c.Assert(docs[0]["x"], gc.Equals, "all is well")

	c.Assert(docs[1]["t"].(time.Time).Sub(t1), gc.Equals, time.Duration(0))
	c.Assert(docs[1]["e"], gc.Equals, envUUID)
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
			c.Assert(log, jc.LessThan, loggo.ERROR)
		}
	}

	// Check that the logsink log file was populated as expected
	logPath := filepath.Join(s.LogDir, "logsink.log")
	logContents, err := ioutil.ReadFile(logPath)
	c.Assert(err, jc.ErrorIsNil)
	line0 := envUUID + " machine-0: 2015-06-01 23:02:01 INFO some.where foo.go:42 all is well\n"
	line1 := envUUID + " machine-0: 2015-06-01 23:02:02 ERROR else.where bar.go:99 oh noes\n"
	c.Assert(string(logContents), gc.Equals, line0+line1)

	// Check the file mode is as expected. This doesn't work on
	// Windows (but this code is very unlikely to run on Windows so
	// it's ok).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(logPath)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info.Mode(), gc.Equals, os.FileMode(0600))
	}
}

func (s *logsinkSuite) dialWebsocket(c *gc.C) *websocket.Conn {
	return s.dialWebsocketInternal(c, s.makeAuthHeader())
}

func (s *logsinkSuite) dialWebsocketInternal(c *gc.C, header http.Header) *websocket.Conn {
	server := s.logsinkURL(c, "wss").String()
	return s.dialWebsocketFromURL(c, server, header)
}

func (s *logsinkSuite) openWebsocketCustomPath(c *gc.C, path string) *bufio.Reader {
	server := s.logsinkURL(c, "wss")
	server.Path = path
	conn := s.dialWebsocketFromURL(c, server.String(), s.makeAuthHeader())
	s.AddCleanup(func(_ *gc.C) { conn.Close() })
	return bufio.NewReader(conn)
}

func (s *logsinkSuite) makeAuthHeader() http.Header {
	header := utils.BasicAuthHeader(s.machineTag.String(), s.password)
	header.Add("X-Juju-Nonce", s.nonce)
	return header
}

type logsinkNoFeatureSuite struct {
	logsinkBaseSuite
}

var _ = gc.Suite(&logsinkNoFeatureSuite{})

func (s *logsinkNoFeatureSuite) TestNoApiWithoutFeatureFlag(c *gc.C) {
	server := s.logsinkURL(c, "wss").String()
	config := s.makeWebsocketConfigFromURL(c, server, nil)
	_, err := websocket.DialConfig(config)
	c.Assert(err, gc.ErrorMatches, ".+/logsink: bad status$")
}
