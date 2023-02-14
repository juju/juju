// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/client/mocks"
	"github.com/juju/juju/api/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

type clientSuite struct {
	Connection *mocks.MockConnection
	Ctrl       *gomock.Controller
}

var _ = gc.Suite(&clientSuite{})

// TODO(jam) 2013-08-27 http://pad.lv/1217282
// Right now most of the direct tests for client.Client behavior are in
// apiserver/client/*_test.go
func (s *clientSuite) SetUpTest(c *gc.C) {
	s.Ctrl = gomock.NewController(c)
	s.Connection = mocks.NewMockConnection(s.Ctrl)
}

func (s *clientSuite) TestWatchDebugLogConnected(c *gc.C) {
	ctrl := gomock.NewController(c)
	connection := mocks.NewMockConnection(ctrl)
	defer ctrl.Finish()
	args := common.DebugLogParams{NoTail: true}
	stream := mocks.NewMockStream(ctrl)
	connection.EXPECT().ConnectStream("/log", args.URLQuery()).Return(stream, nil)
	cl := client.NewClientFromAPIConnection(connection)

	// Use the no tail option so we don't try to start a tailing cursor
	// on the oplog when there is no oplog configured in mongo as the tests
	// don't set up mongo in replicaset mode.
	messages, err := cl.WatchDebugLog(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(messages, gc.NotNil)
}

func (s *clientSuite) TestConnectStreamRequiresSlashPathPrefix(c *gc.C) {
	s.Connection.EXPECT().APICall("WebsocketDial", nil, "foo", nil, nil, &params.Error{Message: "cannot make API path from non-slash-prefixed path \"foo\""})
	reader, err := s.Connection.ConnectStream("foo", nil)
	c.Assert(err, gc.ErrorMatches, `cannot make API path from non-slash-prefixed path "foo"`)
	c.Assert(reader, gc.Equals, nil)
}

func (s *clientSuite) TestConnectStreamErrorBadConnection(c *gc.C) {
	s.Connection.EXPECT().APICall("WebsocketDial", nil, "/", nil, nil, fmt.Errorf("bad connection"))
	reader, err := s.Connection.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "bad connection")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorNoData(c *gc.C) {
	s.Connection.EXPECT().APICall("WebsocketDial", nil, "/", nil,
		api.NewFakeStreamReader(&bytes.Buffer{}),
		&params.Error{Message: "unable to read initial response: EOF"})
	reader, err := s.Connection.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: EOF")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorBadData(c *gc.C) {
	s.Connection.EXPECT().APICall("WebsocketDial", nil, "/", nil,
		api.NewFakeStreamReader(strings.NewReader("junk\n")),
		&params.Error{Message: "unable to unmarshal initial response"})
	reader, err := s.Connection.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to unmarshal initial response: .*")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorReadError(c *gc.C) {
	err := fmt.Errorf("bad read")
	s.Connection.EXPECT().APICall("WebsocketDial", nil, "/", nil,
		api.NewFakeStreamReader(&badReader{err}),
		&params.Error{Message: "unable to read initial response: bad read"})
	reader, err := s.Connection.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: bad read")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectControllerStreamRejectsRelativePaths(c *gc.C) {
	reader, err := s.Connection.ConnectControllerStream("foo", nil, nil)
	c.Assert(err, gc.ErrorMatches, `path "foo" is not absolute`)
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectControllerStreamRejectsModelPaths(c *gc.C) {
	reader, err := s.Connection.ConnectControllerStream("/model/foo", nil, nil)
	c.Assert(err, gc.ErrorMatches, `path "/model/foo" is model-specific`)
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectControllerStreamAppliesHeaders(c *gc.C) {
	catcher := api.UrlCatcher{}
	headers := http.Header{}
	headers.Add("thomas", "cromwell")
	headers.Add("anne", "boleyn")
	s.Connection.EXPECT().APICall("WebsocketDial", catcher.RecordLocation, "", nil, nil, nil)

	_, err := s.Connection.ConnectControllerStream("/something", nil, headers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(catcher.Headers().Get("thomas"), gc.Equals, "cromwell")
	c.Assert(catcher.Headers().Get("anne"), gc.Equals, "boleyn")
}

func (s *clientSuite) TestWatchDebugLogParamsEncoded(c *gc.C) {
	defer s.Ctrl.Finish()
	catcher := api.UrlCatcher{}
	s.Connection.EXPECT().APICall("WebsocketDial", catcher.RecordLocation, "", nil, nil, nil)

	params := common.DebugLogParams{
		IncludeEntity: []string{"a", "b"},
		IncludeModule: []string{"c", "d"},
		IncludeLabel:  []string{"e", "f"},
		ExcludeEntity: []string{"g", "h"},
		ExcludeModule: []string{"i", "j"},
		ExcludeLabel:  []string{"k", "l"},
		Limit:         100,
		Backlog:       200,
		Level:         loggo.ERROR,
		Replay:        true,
		NoTail:        true,
		StartTime:     time.Date(2016, 11, 30, 11, 48, 0, 100, time.UTC),
	}

	client := client.NewClientFromAPIConnection(s.Connection)
	_, err := client.WatchDebugLog(params)
	c.Assert(err, jc.ErrorIsNil)

	connectURL, err := url.Parse(catcher.Location())
	c.Assert(err, jc.ErrorIsNil)

	values := connectURL.Query()
	c.Assert(values, jc.DeepEquals, url.Values{
		"includeEntity": params.IncludeEntity,
		"includeModule": params.IncludeModule,
		"includeLabel":  params.IncludeLabel,
		"excludeEntity": params.ExcludeEntity,
		"excludeModule": params.ExcludeModule,
		"excludeLabel":  params.ExcludeLabel,
		"maxLines":      {"100"},
		"backlog":       {"200"},
		"level":         {"ERROR"},
		"replay":        {"true"},
		"noTail":        {"true"},
		"startTime":     {"2016-11-30T11:48:00.0000001Z"},
	})
}

func (s *clientSuite) TestConnectStreamAtUUIDPath(c *gc.C) {
	catcher := api.UrlCatcher{}
	s.Connection.EXPECT().APICall("WebsocketDial", catcher.RecordLocation, "/path", "test-model-uuid", nil, nil)

	mt, _ := s.Connection.ModelTag()
	info := &api.Info{ModelTag: mt}
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()

	_, err = s.Connection.ConnectStream("/path", nil)
	c.Assert(err, jc.ErrorIsNil)
	connectURL, err2 := url.Parse(catcher.Location())
	c.Assert(err2, jc.ErrorIsNil)
	c.Assert(connectURL.Path, gc.Matches, fmt.Sprintf("/model/%s/path", "test-model-uuid"))
}

func (s *clientSuite) TestOpenUsesModelUUIDPaths(c *gc.C) {
	defer s.Ctrl.Finish()
	// Passing in the correct model UUID should work
	mt, _ := s.Connection.ModelTag()
	info := &api.Info{ModelTag: mt}
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	apistate.Close()

	// Passing in an unknown model UUID should fail with a known error
	info.ModelTag = names.NewModelTag("1eaf1e55-70ad-face-b007-70ad57001999")
	apistate, err = api.Open(info, api.DialOpts{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown model: "1eaf1e55-70ad-face-b007-70ad57001999"`,
		Code:    "model not found",
	})
	c.Check(err, jc.Satisfies, params.IsCodeModelNotFound)
	c.Assert(apistate, gc.IsNil)
}

func (s *clientSuite) TestAbortCurrentUpgrade(c *gc.C) {
	defer s.Ctrl.Finish()

	someErr := errors.New("random")
	mockFacadeCaller := basemocks.NewMockFacadeCaller(s.Ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("AbortCurrentUpgrade", nil, nil).Return(someErr)
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

// badReader raises err when Read is called.
type badReader struct {
	err error
}

func (r *badReader) Read(p []byte) (n int, err error) {
	return 0, r.err
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
