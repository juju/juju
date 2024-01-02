// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-querystring/query"
	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/websocket"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type LogStreamIntSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LogStreamIntSuite{})

func (s *LogStreamIntSuite) TestParamConversion(c *gc.C) {
	cfg := params.LogStreamConfig{
		Sink:               "spam",
		MaxLookbackRecords: 100,
	}
	req := s.newReq(c, cfg)

	stub := &testing.Stub{}
	source := &stubSource{stub: stub}
	source.ReturnGetStart = 10
	handler := logStreamEndpointHandler{
		stopCh:    nil,
		newSource: source.newSource,
	}

	_, err := handler.newLogStreamRequestHandler(nil, req, clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)

	stub.CheckCallNames(c, "newSource", "getStart", "newTailer")
	stub.CheckCall(c, 1, "getStart", "spam")
	stub.CheckCall(c, 2, "newTailer", corelogger.LogTailerParams{
		StartTime:    time.Unix(10, 0),
		InitialLines: 100,
	})
}

type mockClock struct {
	clock.Clock
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func (s *LogStreamIntSuite) TestParamStartTruncate(c *gc.C) {
	cfg := params.LogStreamConfig{
		Sink:                "spam",
		MaxLookbackDuration: "2h",
	}
	req := s.newReq(c, cfg)

	stub := &testing.Stub{}
	source := &stubSource{stub: stub}
	source.ReturnGetStart = 0
	handler := logStreamEndpointHandler{
		stopCh:    nil,
		newSource: source.newSource,
	}

	now := time.Now()
	clock := &mockClock{now: now}

	_, err := handler.newLogStreamRequestHandler(nil, req, clock)
	c.Assert(err, jc.ErrorIsNil)

	stub.CheckCallNames(c, "newSource", "getStart", "newTailer")
	stub.CheckCall(c, 1, "getStart", "spam")
	stub.CheckCall(c, 2, "newTailer", corelogger.LogTailerParams{
		StartTime: now.Add(-2 * time.Hour),
	})
}

func (s *LogStreamIntSuite) TestFullRequest(c *gc.C) {

	// Create test data: i.e. log records for tailing...
	logs := []corelogger.LogRecord{{
		ID:        10,
		ModelUUID: "deadbeef-...",
		Version:   version.Current,
		Time:      time.Date(2015, 6, 19, 15, 34, 37, 0, time.UTC),
		Entity:    "machine-99",
		Module:    "some.where",
		Location:  "code.go:42",
		Level:     loggo.INFO,
		Message:   "stuff happened",
	}, {
		ID:        20,
		ModelUUID: "deadbeef-...",
		Version:   version.Current,
		Time:      time.Date(2015, 6, 19, 15, 36, 40, 0, time.UTC),
		Entity:    "unit-foo-2",
		Module:    "else.where",
		Location:  "go.go:22",
		Level:     loggo.ERROR,
		Message:   "whoops",
	}}

	// ...and transform them into the records we expect to see.
	// (It would be better to create those records explicitly --
	// this is altogether too close to a violation of don't-copy-
	// the-implementation-into-the-tests.)
	var expected []params.LogStreamRecords
	for _, rec := range logs {
		expected = append(expected, params.LogStreamRecords{
			Records: []params.LogStreamRecord{{
				ID:        rec.ID,
				ModelUUID: rec.ModelUUID,
				Entity:    rec.Entity,
				Version:   version.Current.String(),
				Timestamp: rec.Time,
				Module:    rec.Module,
				Location:  rec.Location,
				Level:     rec.Level.String(),
				Message:   rec.Message,
			}}})
	}

	// Create a tailer that will supply the source log records,
	// defined above, to the request handler we're (primarily)
	// testing, as set up in the next block; and create the
	// http request that the handler's execution is (purportedly)
	// caused by.
	tailer := &stubLogTailer{stub: &testing.Stub{}}
	tailer.ReturnLogs = tailer.newChannel(logs)
	req := s.newReq(c, params.LogStreamConfig{
		Sink: "eggs",
	})

	// Start the websocket server, which apes expected apiserver
	// behaviour by calling `initStream` and then handing over to
	// the `logStreamRequestHandler` as configured. That is to say:
	// this server callback holds everything that's *actually* being
	// tested here.
	serverDone := make(chan struct{})
	abortServer := make(chan struct{})
	client := newWebsocketServer(c, func(conn *websocket.Conn) {
		defer close(serverDone)
		defer conn.Close()

		conn.SendInitialErrorV0(nil)
		handler := &logStreamRequestHandler{
			conn:   conn.Conn,
			req:    req,
			tailer: tailer,
		}
		handler.serveWebsocket(abortServer)
	})
	defer waitFor(c, serverDone)
	defer close(abortServer)

	// Stream out the results from the client. This whole block is
	// just scaffolding to get the results back out on the records
	// channel, and should probably be replaced by something more
	// direct. The goroutine is needed here because the JSON Receive
	// call will block waiting on the server to send the next message.
	clientDone := make(chan struct{})
	records := make(chan params.LogStreamRecords)
	go func() {
		defer close(clientDone)

		var result params.ErrorResult
		err := client.ReadJSON(&result)
		ok := c.Check(err, jc.ErrorIsNil)
		if ok && c.Check(result, jc.DeepEquals, params.ErrorResult{}) {
			for {
				var apiRec params.LogStreamRecords
				err = client.ReadJSON(&apiRec)
				if err != nil {
					break
				}
				records <- apiRec
			}
		}

		c.Logf("client stopped: %v", err)
		if gorillaws.IsCloseError(err,
			gorillaws.CloseNormalClosure,
			gorillaws.CloseGoingAway,
			gorillaws.CloseNoStatusReceived) {
			return // this is fine
		}
		if _, ok := err.(*net.OpError); ok {
			return // so is this, probably
		}
		if err != nil && strings.Contains(err.Error(), "use of closed network connection") {
			return // and so is this.
		}
		// anything else is a problem
		c.Check(err, jc.ErrorIsNil)
	}()
	defer waitFor(c, clientDone)
	defer client.Close()

	// Check the client produces the expected records. (This is the
	// actual *test* bit of the test, vs the scaffolding that
	// accounts for just about everything else.)
	for i, expectedRec := range expected {
		c.Logf("trying #%d: %#v", i, expectedRec)
		select {
		case apiRec := <-records:
			c.Check(apiRec, jc.DeepEquals, expectedRec)
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for log record")
		}
	}

	// Wait a moment to be sure there aren't any extra records.
	select {
	case apiRec := <-records:
		c.Errorf("got unexpected record: %#v", apiRec)
	case <-time.After(coretesting.ShortWait):
		// All good, let the defers handle teardown.
	}
}

func (s *LogStreamIntSuite) newReq(c *gc.C, cfg params.LogStreamConfig) *http.Request {
	attrs, err := query.Values(cfg)
	c.Assert(err, jc.ErrorIsNil)
	URL, err := url.Parse("https://a.b.c/logstream")
	c.Assert(err, jc.ErrorIsNil)
	URL.RawQuery = attrs.Encode()
	req, err := http.NewRequest("GET", URL.String(), nil)
	c.Assert(err, jc.ErrorIsNil)
	return req
}

type stubSource struct {
	stub *testing.Stub

	ReturnGetStart  int64
	ReturnNewTailer corelogger.LogTailer
}

func (s *stubSource) newSource(req *http.Request) (logStreamSource, state.PoolHelper, error) {
	s.stub.AddCall("newSource", req)
	if err := s.stub.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	ph := apiservertesting.StubPoolHelper{StubRelease: func() bool {
		s.stub.AddCall("close")
		return false
	}}
	return s, ph, nil
}

func (s *stubSource) getStart(sink string) (time.Time, error) {
	s.stub.AddCall("getStart", sink)
	if err := s.stub.NextErr(); err != nil {
		return time.Time{}, errors.Trace(err)
	}

	return time.Unix(s.ReturnGetStart, 0), nil
}

func (s *stubSource) newTailer(args corelogger.LogTailerParams) (corelogger.LogTailer, error) {
	s.stub.AddCall("newTailer", args)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnNewTailer, nil
}

type stubLogTailer struct {
	corelogger.LogTailer
	stub *testing.Stub

	ReturnLogs <-chan *corelogger.LogRecord
}

func (s *stubLogTailer) newChannel(logs []corelogger.LogRecord) <-chan *corelogger.LogRecord {
	ch := make(chan *corelogger.LogRecord)
	go func() {
		for i := range logs {
			rec := logs[i]
			ch <- &rec
		}
	}()
	return ch
}

func (s *stubLogTailer) Logs() <-chan *corelogger.LogRecord {
	s.stub.AddCall("Logs")
	s.stub.NextErr() // pop one off

	return s.ReturnLogs
}

func (s *stubLogTailer) Err() error {
	s.stub.AddCall("Err")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type testStreamHandler struct {
	handler func(*websocket.Conn)
}

func (h *testStreamHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	websocket.Serve(w, req, h.handler)
}

func newWebsocketServer(c *gc.C, h func(*websocket.Conn)) *gorillaws.Conn {
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, jc.ErrorIsNil)
	port := listener.Addr().(*net.TCPAddr).Port

	go http.Serve(listener, &testStreamHandler{h})

	return newWebsocketClient(c, port)
}

func newWebsocketClient(c *gc.C, port int) *gorillaws.Conn {
	address := fmt.Sprintf("ws://localhost:%d/", port)
	client, _, err := gorillaws.DefaultDialer.Dial(address, nil)
	if err == nil {
		return client
	}

	timeoutCh := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeoutCh:
			c.Fatalf("unable to connect to %s", address)
		case <-time.After(coretesting.ShortWait):
		}

		client, _, err = gorillaws.DefaultDialer.Dial(address, nil)
		if err != nil {
			c.Logf("failed attempt to connect to %s", address)
			continue
		}
		return client
	}
}

func waitFor(c *gc.C, done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("channel never closed")
	}
}
