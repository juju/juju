// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/go-querystring/query"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
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
		AllModels: true,
		Sink:      "spam",
	}
	req := s.newReq(c, cfg)

	stub := &testing.Stub{}
	source := &stubSource{stub: stub}
	source.ReturnGetStart = 10
	handler := logStreamEndpointHandler{
		stopCh:    nil,
		newSource: source.newSource,
	}

	reqHandler, err := handler.newLogStreamRequestHandler(req)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(reqHandler.sendModelUUID, jc.IsTrue)
	stub.CheckCallNames(c, "newSource", "getStart", "newTailer")
	stub.CheckCall(c, 1, "getStart", "spam", true)
	stub.CheckCall(c, 2, "newTailer", &state.LogTailerParams{
		StartID:   10,
		AllModels: true,
	})
}

func (s *LogStreamIntSuite) TestFullRequest(c *gc.C) {
	cfg := params.LogStreamConfig{
		AllModels: true,
		Sink:      "eggs",
	}
	req := s.newReq(c, cfg)
	stub := &testing.Stub{}
	source := &stubSource{stub: stub}
	source.ReturnGetStart = 10
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
	var expected []params.LogStreamRecord
	for _, rec := range logs {
		expected = append(expected, params.LogStreamRecord{
			ID:        rec.ID,
			ModelUUID: rec.ModelUUID,
			Entity:    rec.Entity.String(),
			Version:   version.Current.String(),
			Timestamp: rec.Time,
			Module:    rec.Module,
			Location:  rec.Location,
			Level:     rec.Level.String(),
			Message:   rec.Message,
		})
	}
	tailer := &stubLogTailer{stub: stub}
	tailer.ReturnLogs = tailer.newChannel(logs)
	source.ReturnNewTailer = tailer
	reqHandler := &logStreamRequestHandler{
		req:           req,
		tailer:        tailer,
		sendModelUUID: true,
	}

	// Start the websocket server.
	stop := make(chan struct{})
	client := newWebsocketServer(c, func(conn *websocket.Conn) {
		defer conn.Close()
		initial := error(nil)
		reqHandler.serveWebsocket(conn, initial, stop)
	})
	defer client.Close()
	defer close(stop)

	// Stream out the results from the client.
	okCh := make(chan params.ErrorResult)
	receivedCh := make(chan params.LogStreamRecord)
	go func() {
		var initial params.ErrorResult
		err := websocket.JSON.Receive(client, &initial)
		c.Assert(err, jc.ErrorIsNil)
		okCh <- initial

		for {
			var apiRec params.LogStreamRecord
			err := websocket.JSON.Receive(client, &apiRec)
			if err == io.EOF {
				break
			}
			c.Assert(err, jc.ErrorIsNil)
			receivedCh <- apiRec
		}
	}()

	// Check the OK message.
	select {
	case initial := <-okCh:
		c.Check(initial, jc.DeepEquals, params.ErrorResult{})
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for OK message")
	}

	// Check the records coming from the client.
	for i, expectedRec := range expected {
		c.Logf("trying #%d: %#v", i, expectedRec)
		select {
		case apiRec := <-receivedCh:
			c.Check(apiRec, jc.DeepEquals, expectedRec)
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for OK message")
		}
	}

	// Make sure there aren't any extras.
	select {
	case apiRec := <-receivedCh:
		c.Errorf("got extra: %#v", apiRec)
	default:
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

func (s *stubSource) newSource(req *http.Request) (logStreamSource, error) {
	s.stub.AddCall("newSource", req)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s, nil
}

func (s *stubSource) getStart(sink string, allModels bool) (int64, error) {
	s.stub.AddCall("getStart", sink, allModels)
	if err := s.stub.NextErr(); err != nil {
		return 0, errors.Trace(err)
	}

	return s.ReturnGetStart, nil
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
