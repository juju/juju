// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/go-querystring/query"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
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
		AllModels:          true,
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

	reqHandler, err := handler.newLogStreamRequestHandler(req, clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(reqHandler.sendModelUUID, jc.IsTrue)
	stub.CheckCallNames(c, "newSource", "getStart", "newTailer")
	stub.CheckCall(c, 1, "getStart", "spam", true)
	stub.CheckCall(c, 2, "newTailer", &state.LogTailerParams{
		StartTime:    time.Unix(10, 0),
		AllModels:    true,
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

	reqHandler, err := handler.newLogStreamRequestHandler(req, clock)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(reqHandler.sendModelUUID, jc.IsFalse)
	stub.CheckCallNames(c, "newSource", "getStart", "newTailer")
	stub.CheckCall(c, 1, "getStart", "spam", false)
	stub.CheckCall(c, 2, "newTailer", &state.LogTailerParams{
		StartTime: now.Add(-2 * time.Hour),
	})
}

func (s *LogStreamIntSuite) TestFullRequest(c *gc.C) {

	// Create test data: i.e. log records for tailing...
	logs := []state.LogRecord{{
		ID:        10,
		ModelUUID: "deadbeef-...",
		Version:   version.Current,
		Time:      time.Date(2015, 6, 19, 15, 34, 37, 0, time.UTC),
		Entity:    names.NewMachineTag("99"),
		Module:    "some.where",
		Location:  "code.go:42",
		Level:     loggo.INFO,
		Message:   "stuff happened",
	}, {
		ID:        20,
		ModelUUID: "deadbeef-...",
		Version:   version.Current,
		Time:      time.Date(2015, 6, 19, 15, 36, 40, 0, time.UTC),
		Entity:    names.NewUnitTag("foo/2"),
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
				Entity:    rec.Entity.String(),
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
		AllModels: true,
		Sink:      "eggs",
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

		stream, err := initStream(conn, nil)
		if !c.Check(err, jc.ErrorIsNil) {
			return
		}
		handler := &logStreamRequestHandler{
			req:           req,
			tailer:        tailer,
			sendModelUUID: true,
		}
		handler.serveWebsocket(conn, stream, abortServer)
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
		err := websocket.JSON.Receive(client, &result)
		ok := c.Check(err, jc.ErrorIsNil)
		if ok && c.Check(result, jc.DeepEquals, params.ErrorResult{}) {
			for {
				var apiRec params.LogStreamRecords
				err = websocket.JSON.Receive(client, &apiRec)
				if err != nil {
					break
				}
				records <- apiRec
			}
		}

		c.Logf("client stopped: %v", err)
		if err == io.EOF {
			return // this is fine
		}
		if _, ok := err.(*net.OpError); ok {
			return // so is this, probably
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
	ReturnNewTailer state.LogTailer
}

func (s *stubSource) newSource(req *http.Request) (logStreamSource, closerFunc, error) {
	s.stub.AddCall("newSource", req)
	if err := s.stub.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	closer := func() error {
		s.stub.AddCall("close")
		return s.stub.NextErr()
	}
	return s, closer, nil
}

func (s *stubSource) getStart(sink string, allModels bool) (time.Time, error) {
	s.stub.AddCall("getStart", sink, allModels)
	if err := s.stub.NextErr(); err != nil {
		return time.Time{}, errors.Trace(err)
	}

	return time.Unix(s.ReturnGetStart, 0), nil
}

func (s *stubSource) newTailer(args *state.LogTailerParams) (state.LogTailer, error) {
	s.stub.AddCall("newTailer", args)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnNewTailer, nil
}

type stubLogTailer struct {
	state.LogTailer
	stub *testing.Stub

	ReturnLogs <-chan *state.LogRecord
}

func (s *stubLogTailer) newChannel(logs []state.LogRecord) <-chan *state.LogRecord {
	ch := make(chan *state.LogRecord)
	go func() {
		for i := range logs {
			rec := logs[i]
			ch <- &rec
		}
	}()
	return ch
}

func (s *stubLogTailer) Logs() <-chan *state.LogRecord {
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

func newWebsocketServer(c *gc.C, h websocket.Handler) *websocket.Conn {
	cfg, err := websocket.NewConfig("ws://localhost:12345/", "http://localhost/")
	c.Assert(err, jc.ErrorIsNil)

	go func() {
		err = http.ListenAndServe(":12345", websocket.Server{
			Config:  *cfg,
			Handler: h,
		})
		c.Assert(err, jc.ErrorIsNil)
	}()

	return newWebsocketClient(c, cfg)
}

func newWebsocketClient(c *gc.C, cfg *websocket.Config) *websocket.Conn {
	client, err := websocket.DialConfig(cfg)
	if err == nil {
		return client
	}

	timeoutCh := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeoutCh:
			c.Assert(err, jc.ErrorIsNil)
		case <-time.After(coretesting.ShortWait):
		}

		client, err = websocket.DialConfig(cfg)
		if _, ok := err.(*websocket.DialError); ok {
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
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
