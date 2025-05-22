// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type clientSuite struct{}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
}

// TODO(jam) 2013-08-27 http://pad.lv/1217282
// Right now most of the direct tests for client.Client behavior are in
// apiserver/client/*_test.go

func (s *clientSuite) TestWebsocketDialWithErrorsJSON(c *tc.C) {
	errorResult := params.ErrorResult{
		Error: apiservererrors.ServerError(errors.New("kablooie")),
	}
	data, err := json.Marshal(errorResult)
	c.Assert(err, tc.ErrorIsNil)
	cw := closeWatcher{Reader: bytes.NewReader(data)}
	d := fakeDialer{
		resp: &http.Response{
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: &cw,
		},
	}
	d.SetErrors(websocket.ErrBadHandshake)
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(stream, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "kablooie")
	c.Assert(cw.closed, tc.Equals, true)
}

func (s *clientSuite) TestWebsocketDialWithErrorsNoJSON(c *tc.C) {
	cw := closeWatcher{Reader: strings.NewReader("wowee zowee")}
	d := fakeDialer{
		resp: &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       &cw,
		},
	}
	d.SetErrors(websocket.ErrBadHandshake)
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(stream, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `wowee zowee \(Not Found\)`)
	c.Assert(cw.closed, tc.Equals, true)
}

func (s *clientSuite) TestWebsocketDialWithErrorsOtherError(c *tc.C) {
	var d fakeDialer
	d.SetErrors(errors.New("jammy pac"))
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(stream, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "jammy pac")
}

func (s *clientSuite) TestWebsocketDialWithErrorsSetsDeadline(c *tc.C) {
	// I haven't been able to find a way to actually test the
	// websocket deadline stream, so instead test that the stream
	// returned from websocketDialWithErrors is actually a
	// DeadlineStream with the expected timeout.
	d := fakeDialer{}
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(err, tc.ErrorIsNil)
	deadlineStream, ok := stream.(*api.DeadlineStream)
	c.Assert(ok, tc.Equals, true)
	c.Assert(deadlineStream.Timeout, tc.Equals, 30*time.Second)
}

type fakeDialer struct {
	testhelpers.Stub

	conn *websocket.Conn
	resp *http.Response
}

func (d *fakeDialer) Dial(url string, header http.Header) (*websocket.Conn, *http.Response, error) {
	d.AddCall("Dial", url, header)
	return d.conn, d.resp, d.NextErr()
}

type closeWatcher struct {
	io.Reader
	closed bool
}

func (c *closeWatcher) Close() error {
	c.closed = true
	return nil
}
