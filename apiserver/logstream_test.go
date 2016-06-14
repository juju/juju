// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type LogStreamIntSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LogStreamIntSuite{})

func (s *LogStreamIntSuite) TestParamConversion(c *gc.C) {
	start := time.Unix(12345, 0)
	cfg := params.LogStreamConfig{
		AllModels: true,
		StartTime: start,
	}
	req := s.newReq(c, cfg)

	var tailerArgs *state.LogTailerParams
	handler := logStreamEndpointHandler{
		stopCh: nil,
		newState: func(*http.Request) (state.LogTailerState, error) {
			return nil, nil
		},
		newTailer: func(_ state.LogTailerState, args *state.LogTailerParams) (state.LogTailer, error) {
			tailerArgs = args
			return nil, nil
		},
	}

	reqHandler, err := handler.newLogStreamRequestHandler(req)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(reqHandler.cfg, jc.DeepEquals, cfg)
	c.Check(tailerArgs, jc.DeepEquals, &state.LogTailerParams{
		StartTime: start,
		AllModels: true,
	})
}

func (s *LogStreamIntSuite) TestFullRequest(c *gc.C) {
	cfg := params.LogStreamConfig{
		AllModels: true,
		StartTime: time.Unix(12345, 0),
	}
	req := s.newReq(c, cfg)
	tailer := &stubLogTailer{}
	tailer.logs = []state.LogRecord{{
		ModelUUID: "deadbeef-...",
		Time:      time.Date(2015, 6, 19, 15, 34, 37, 0, time.UTC),
		Entity:    "machine-99",
		Module:    "some.where",
		Location:  "code.go:42",
		Level:     loggo.INFO,
		Message:   "stuff happened",
	}, {
		ModelUUID: "deadbeef-...",
		Time:      time.Date(2015, 6, 19, 15, 36, 40, 0, time.UTC),
		Entity:    "unit-foo-2",
		Module:    "else.where",
		Location:  "go.go:22",
		Level:     loggo.ERROR,
		Message:   "whoops",
	}}
	var expected []params.LogStreamRecord
	for _, rec := range tailer.logs {
		expected = append(expected, params.LogStreamRecord{
			ModelUUID: rec.ModelUUID,
			Time:      rec.Time,
			Module:    rec.Module,
			Location:  rec.Location,
			Level:     rec.Level.String(),
			Message:   rec.Message,
		})
	}
	reqHandler := &logStreamRequestHandler{
		req:    req,
		cfg:    cfg,
		tailer: tailer,
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
	query := make(url.Values)
	cfg.Apply(query)
	URL, err := url.Parse("https://a.b.c/logstream")
	c.Assert(err, jc.ErrorIsNil)
	URL.RawQuery = query.Encode()
	req, err := http.NewRequest("GET", URL.String(), nil)
	c.Assert(err, jc.ErrorIsNil)
	return req
}

type stubLogTailer struct {
	state.LogTailer
	logs []state.LogRecord

	logsCh <-chan *state.LogRecord
}

func (t *stubLogTailer) newChannel() <-chan *state.LogRecord {
	ch := make(chan *state.LogRecord)
	go func() {
		for i := range t.logs {
			rec := t.logs[i]
			ch <- &rec
		}
	}()
	return ch
}

func (t *stubLogTailer) Logs() <-chan *state.LogRecord {
	if t.logsCh == nil {
		t.logsCh = t.newChannel()
	}
	return t.logsCh
}

func (t *stubLogTailer) Err() error {
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
