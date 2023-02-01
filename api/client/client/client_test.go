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

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

type clientSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&clientSuite{})

// TODO(jam) 2013-08-27 http://pad.lv/1217282
// Right now most of the direct tests for client.Client behavior are in
// apiserver/client/*_test.go
func (s *clientSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *clientSuite) TestWatchDebugLogConnected(c *gc.C) {
	client := client.NewClient(s.APIState)
	// Use the no tail option so we don't try to start a tailing cursor
	// on the oplog when there is no oplog configured in mongo as the tests
	// don't set up mongo in replicaset mode.
	messages, err := client.WatchDebugLog(common.DebugLogParams{NoTail: true})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(messages, gc.NotNil)
}

func (s *clientSuite) TestConnectStreamRequiresSlashPathPrefix(c *gc.C) {
	reader, err := s.APIState.ConnectStream("foo", nil)
	c.Assert(err, gc.ErrorMatches, `cannot make API path from non-slash-prefixed path "foo"`)
	c.Assert(reader, gc.Equals, nil)
}

func (s *clientSuite) TestConnectStreamErrorBadConnection(c *gc.C) {
	s.PatchValue(&api.WebsocketDial, func(_ api.WebsocketDialer, _ string, _ http.Header) (base.Stream, error) {
		return nil, fmt.Errorf("bad connection")
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "bad connection")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorNoData(c *gc.C) {
	s.PatchValue(&api.WebsocketDial, func(_ api.WebsocketDialer, _ string, _ http.Header) (base.Stream, error) {
		return api.NewFakeStreamReader(&bytes.Buffer{}), nil
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: EOF")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorBadData(c *gc.C) {
	s.PatchValue(&api.WebsocketDial, func(_ api.WebsocketDialer, _ string, _ http.Header) (base.Stream, error) {
		return api.NewFakeStreamReader(strings.NewReader("junk\n")), nil
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to unmarshal initial response: .*")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectStreamErrorReadError(c *gc.C) {
	s.PatchValue(&api.WebsocketDial, func(_ api.WebsocketDialer, _ string, _ http.Header) (base.Stream, error) {
		err := fmt.Errorf("bad read")
		return api.NewFakeStreamReader(&badReader{err}), nil
	})
	reader, err := s.APIState.ConnectStream("/", nil)
	c.Assert(err, gc.ErrorMatches, "unable to read initial response: bad read")
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectControllerStreamRejectsRelativePaths(c *gc.C) {
	reader, err := s.APIState.ConnectControllerStream("foo", nil, nil)
	c.Assert(err, gc.ErrorMatches, `path "foo" is not absolute`)
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectControllerStreamRejectsModelPaths(c *gc.C) {
	reader, err := s.APIState.ConnectControllerStream("/model/foo", nil, nil)
	c.Assert(err, gc.ErrorMatches, `path "/model/foo" is model-specific`)
	c.Assert(reader, gc.IsNil)
}

func (s *clientSuite) TestConnectControllerStreamAppliesHeaders(c *gc.C) {
	catcher := api.UrlCatcher{}
	headers := http.Header{}
	headers.Add("thomas", "cromwell")
	headers.Add("anne", "boleyn")
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

	_, err := s.APIState.ConnectControllerStream("/something", nil, headers)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(catcher.Headers().Get("thomas"), gc.Equals, "cromwell")
	c.Assert(catcher.Headers().Get("anne"), gc.Equals, "boleyn")
}

func (s *clientSuite) TestWatchDebugLogParamsEncoded(c *gc.C) {
	catcher := api.UrlCatcher{}
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

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

	client := client.NewClient(s.APIState)
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
	s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	info := s.APIInfo(c)
	info.ModelTag = model.ModelTag()
	apistate, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apistate.Close()
	_, err = apistate.ConnectStream("/path", nil)
	c.Assert(err, jc.ErrorIsNil)
	connectURL, err := url.Parse(catcher.Location())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(connectURL.Path, gc.Matches, fmt.Sprintf("/model/%s/path", model.UUID()))
}

func (s *clientSuite) TestOpenUsesModelUUIDPaths(c *gc.C) {
	info := s.APIInfo(c)

	// Passing in the correct model UUID should work
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	info.ModelTag = model.ModelTag()
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
	cl := client.NewClient(s.APIState)
	someErr := errors.New("random")
	cleanup := client.PatchClientFacadeCall(cl,
		func(request string, args interface{}, response interface{}) error {
			c.Assert(request, gc.Equals, "AbortCurrentUpgrade")
			c.Assert(args, gc.IsNil)
			c.Assert(response, gc.IsNil)
			return someErr
		},
	)
	defer cleanup()

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
