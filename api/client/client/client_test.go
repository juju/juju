// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/client"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

type clientSuite struct{}

var _ = gc.Suite(&clientSuite{})

// TODO(jam) 2013-08-27 http://pad.lv/1217282
// Right now most of the direct tests for client.Client behavior are in
// apiserver/client/*_test.go

func (s *clientSuite) TestAbortCurrentUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	someErr := errors.New("random")
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AbortCurrentUpgrade", nil, nil).Return(someErr)
	cl := client.NewClientFromFacadeCaller(mockFacadeCaller)

	err := cl.AbortCurrentUpgrade()
	c.Assert(err, gc.Equals, someErr) // Confirms that the correct facade was called
}

func (s *clientSuite) TestWebsocketDialWithErrorsJSON(c *gc.C) {
	errorResult := params.ErrorResult{
		Error: apiservererrors.ServerError(errors.New("kablooie")),
	}
	data, err := json.Marshal(errorResult)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(stream, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "kablooie")
	c.Assert(cw.closed, gc.Equals, true)
}

func (s *clientSuite) TestWebsocketDialWithErrorsNoJSON(c *gc.C) {
	cw := closeWatcher{Reader: strings.NewReader("wowee zowee")}
	d := fakeDialer{
		resp: &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       &cw,
		},
	}
	d.SetErrors(websocket.ErrBadHandshake)
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(stream, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `wowee zowee \(Not Found\)`)
	c.Assert(cw.closed, gc.Equals, true)
}

func (s *clientSuite) TestWebsocketDialWithErrorsOtherError(c *gc.C) {
	var d fakeDialer
	d.SetErrors(errors.New("jammy pac"))
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(stream, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "jammy pac")
}

func (s *clientSuite) TestWebsocketDialWithErrorsSetsDeadline(c *gc.C) {
	// I haven't been able to find a way to actually test the
	// websocket deadline stream, so instead test that the stream
	// returned from websocketDialWithErrors is actually a
	// DeadlineStream with the expected timeout.
	d := fakeDialer{}
	stream, err := api.WebsocketDialWithErrors(&d, "something", nil)
	c.Assert(err, jc.ErrorIsNil)
	deadlineStream, ok := stream.(*api.DeadlineStream)
	c.Assert(ok, gc.Equals, true)
	c.Assert(deadlineStream.Timeout, gc.Equals, 30*time.Second)
}

type fakeDialer struct {
	testing.Stub

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
