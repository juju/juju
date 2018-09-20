// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/websocket/websockettest"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

type logtransferSuite struct {
	apiserverBaseSuite
	userTag         names.UserTag
	password        string
	machineTag      names.MachineTag
	machinePassword string
	logs            loggo.TestWriter
	url             string
}

var _ = gc.Suite(&logtransferSuite{})

func (s *logtransferSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)
	s.password = "jabberwocky"
	u := s.Factory.MakeUser(c, &factory.UserParams{Password: s.password})
	s.userTag = u.Tag().(names.UserTag)
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "nonce",
	})
	s.machineTag = m.Tag().(names.MachineTag)
	s.machinePassword = password
	s.setUserAccess(c, permission.SuperuserAccess)

	url := s.URL("/migrate/logtransfer", url.Values{
		"jujuclientversion": {version.Current.String()},
	})
	url.Scheme = "wss"
	s.url = url.String()

	s.logs.Clear()
	writer := loggo.NewMinimumLevelWriter(&s.logs, loggo.INFO)
	c.Assert(loggo.RegisterWriter("logsink-tests", writer), jc.ErrorIsNil)
}

func (s *logtransferSuite) makeAuthHeader() http.Header {
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	header.Add(params.MigrationModelHTTPHeader, s.State.ModelUUID())
	return header
}

func (s *logtransferSuite) dialWebsocket(c *gc.C) *websocket.Conn {
	return s.dialWebsocketInternal(c, s.makeAuthHeader())
}

func (s *logtransferSuite) dialWebsocketInternal(c *gc.C, header http.Header) *websocket.Conn {
	conn, _, err := dialWebsocketFromURL(c, s.url, header)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(_ *gc.C) { conn.Close() })
	return conn
}

func (s *logtransferSuite) checkAuthFails(c *gc.C, header http.Header, code int, message string) {
	_, resp, err := dialWebsocketFromURL(c, s.url, header)
	c.Assert(err, gc.Equals, websocket.ErrBadHandshake)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, code)
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(body), gc.Matches, message+"\n")
}

func (s *logtransferSuite) TestRejectsMissingModelHeader(c *gc.C) {
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	ws := s.dialWebsocketInternal(c, header)
	websockettest.AssertJSONError(c, ws, `initialising migration logsink session: unknown model: ""`)
	websockettest.AssertWebsocketClosed(c, ws)
}

func (s *logtransferSuite) TestRejectsBadMigratingModelUUID(c *gc.C) {
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	header.Add(params.MigrationModelHTTPHeader, "does-not-exist")
	ws := s.dialWebsocketInternal(c, header)
	websockettest.AssertJSONError(c, ws, `initialising migration logsink session: unknown model: "does-not-exist"`)
	websockettest.AssertWebsocketClosed(c, ws)
}

func (s *logtransferSuite) TestRejectsInvalidVersion(c *gc.C) {
	url := s.URL("/migrate/logtransfer", url.Values{"jujuclientversion": {"blah"}})
	url.Scheme = "wss"
	conn, _, err := dialWebsocketFromURL(c, url.String(), s.makeAuthHeader())
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	websockettest.AssertJSONError(c, conn, `^initialising migration logsink session: invalid jujuclientversion "blah".*`)
	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *logtransferSuite) TestRejectsMachineLogins(c *gc.C) {
	header := utils.BasicAuthHeader(s.machineTag.String(), s.machinePassword)
	header.Add(params.MachineNonceHeader, "nonce")
	s.checkAuthFails(c, header, http.StatusForbidden, "authorization failed: machine 0 is not a user")
}

func (s *logtransferSuite) TestRejectsBadPasword(c *gc.C) {
	header := utils.BasicAuthHeader(s.userTag.String(), "wrong")
	header.Add(params.MigrationModelHTTPHeader, s.State.ModelUUID())
	s.checkAuthFails(c, header, http.StatusUnauthorized, "authentication failed: invalid entity name or password")
}

func (s *logtransferSuite) TestRequiresSuperUser(c *gc.C) {
	s.setUserAccess(c, permission.LoginAccess)
	s.checkAuthFails(c, s.makeAuthHeader(), http.StatusForbidden, "authorization failed: user .* is not a controller admin")
}

func (s *logtransferSuite) TestRequiresMigrationModeNone(c *gc.C) {
	s.setMigrationMode(c, state.MigrationModeImporting)
	ws := s.dialWebsocket(c)
	websockettest.AssertJSONError(c, ws, `initialising migration logsink session: model migration mode is "importing" instead of ""`)
	websockettest.AssertWebsocketClosed(c, ws)
}

func (s *logtransferSuite) TestLogging(c *gc.C) {
	conn := s.dialWebsocket(c)

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	err := conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	})
	c.Assert(err, jc.ErrorIsNil)

	t1 := time.Date(2015, time.June, 1, 23, 2, 2, 0, time.UTC)
	err = conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-101",
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
	c.Assert(docs[0]["t"], gc.Equals, t0.UnixNano())
	c.Assert(docs[0]["n"], gc.Equals, "machine-23")
	c.Assert(docs[0]["m"], gc.Equals, "some.where")
	c.Assert(docs[0]["l"], gc.Equals, "foo.go:42")
	c.Assert(docs[0]["v"], gc.Equals, int(loggo.INFO))
	c.Assert(docs[0]["x"], gc.Equals, "all is well")

	c.Assert(docs[1]["t"], gc.Equals, t1.UnixNano())
	c.Assert(docs[1]["n"], gc.Equals, "machine-101")
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
}

func (s *logtransferSuite) TestTracksLastSentLogTime(c *gc.C) {
	conn := s.dialWebsocket(c)

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	tracker := state.NewLastSentLogTracker(s.State, s.State.ModelUUID(), "migration-logtransfer")
	defer tracker.Close()

	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	err := conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	})
	c.Assert(err, jc.ErrorIsNil)

	// First message time is tracked.
	assertTrackerTime(c, tracker, t0)

	// Doesn't track anything more until a log message 2 mins later.
	t1 := t0.Add(2*time.Minute - 1*time.Nanosecond)
	err = conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t1,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "still good",
	})
	c.Assert(err, jc.ErrorIsNil)

	// No change
	assertTrackerTime(c, tracker, t0)

	t2 := t1.Add(1 * time.Nanosecond)
	err = conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t2,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "nae bather",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Updated
	assertTrackerTime(c, tracker, t2)

	t3 := t2.Add(1 * time.Nanosecond)
	err = conn.WriteJSON(&params.LogRecord{
		Entity:   "machine-23",
		Time:     t3,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "sweet as",
	})
	c.Assert(err, jc.ErrorIsNil)

	// No change,
	assertTrackerTime(c, tracker, t2)

	err = conn.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Latest is saved when connection is closed.
	assertTrackerTime(c, tracker, t3)
}

func assertTrackerTime(c *gc.C, tracker *state.LastSentLogTracker, expected time.Time) {
	var timestamp int64
	var err error
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, timestamp, err = tracker.Get()
		if err != nil && errors.Cause(err) != state.ErrNeverForwarded {
			c.Assert(err, jc.ErrorIsNil)
		}
		if err == nil && timestamp == expected.UnixNano() {
			return
		}
	}
	c.Fatalf("tracker never set to %d - last seen was %d (err: %v)", expected.UnixNano(), timestamp, err)
}

func (s *logtransferSuite) setUserAccess(c *gc.C, level permission.Access) {
	_, err := s.State.SetUserAccess(s.userTag, s.State.ControllerTag(), level)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *logtransferSuite) setMigrationMode(c *gc.C, mode state.MigrationMode) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(mode)
	c.Assert(err, jc.ErrorIsNil)
}
