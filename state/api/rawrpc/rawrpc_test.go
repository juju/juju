// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rawrpc_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api/rawrpc"
	coretesting "github.com/juju/juju/testing"
)

type rawrpcSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&rawrpcSuite{})

//---------------------------
// test helpers

type FakeData struct {
	Raw io.Reader
	Err error
}

func NewFakeData(data string) *FakeData {
	raw := bytes.NewBufferString(data)
	return &FakeData{Raw: raw}
}

func InvalidData(msg string) *FakeData {
	return &FakeData{Err: fmt.Errorf(msg)}
}

func (d *FakeData) Read(p []byte) (n int, err error) {
	if d.Err != nil {
		return 0, d.Err
	}
	return d.Raw.Read(p)
}

func (d *FakeData) Close() error {
	return nil
}

type FakeResult struct{ Error string }

func (r *FakeResult) Err() error {
	if r.Error != "" {
		return fmt.Errorf(r.Error)
	}

	return nil
}

type FakeHTTPClient struct {
	Code int
	Data io.Reader
	Err  error
}

func (c FakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if c.Err != nil {
		return nil, c.Err
	}

	code := c.Code
	if code <= 0 {
		code = http.StatusOK
	}

	data := c.Data
	if data == nil {
		data = bytes.NewBufferString("")
	}

	resp := http.Response{
		StatusCode: code,
		Body:       &FakeData{Raw: data},
	}

	return &resp, nil
}

//---------------------------
// UnpackJSON() tests

func (s *rawrpcSuite) TestUnpackJSONValid(c *gc.C) {
	var result FakeResult
	data := bytes.NewBufferString(`{"Error": ""}`)
	err := rawrpc.UnpackJSON(data, &result)

	c.Assert(err, gc.IsNil)
}

func (s *rawrpcSuite) TestUnpackJSONMissingErrorResult(c *gc.C) {
	data := bytes.NewBufferString("")
	err := rawrpc.UnpackJSON(data, nil)

	c.Assert(err, gc.IsNil)
}

func (s *rawrpcSuite) TestUnpackJSONNotErrorResult(c *gc.C) {
	var result struct{}
	data := bytes.NewBufferString("{}")
	err := rawrpc.UnpackJSON(data, &result)

	c.Assert(err, gc.IsNil)
}

func (s *rawrpcSuite) TestUnpackJSONUnreadableData(c *gc.C) {
	var result struct{}
	data := InvalidData("invalid!")
	err := rawrpc.UnpackJSON(data, &result)

	c.Assert(err, gc.ErrorMatches, "could not read response data: .*")
}

func (s *rawrpcSuite) TestUnpackJSONUnpackableData(c *gc.C) {
	var result struct{}
	data := bytes.NewBufferString("not valid JSON")
	err := rawrpc.UnpackJSON(data, &result)

	c.Assert(err, gc.ErrorMatches, "could not unpack response data: .*")
}

func (s *rawrpcSuite) TestUnpackJSONFailed(c *gc.C) {
	var result FakeResult
	data := bytes.NewBufferString(`{"Error": "failed!"}`)
	err := rawrpc.UnpackJSON(data, &result)

	c.Assert(err, gc.ErrorMatches, "request failed on server: .*")
}

//---------------------------
// Send() tests

func (s *rawrpcSuite) TestSendValidNoData(c *gc.C) {
	client := FakeHTTPClient{}
	resp, err := rawrpc.Send(&client, nil, nil)
	data, _ := ioutil.ReadAll(resp.Body)

	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "")
}

func (s *rawrpcSuite) TestSendValidData(c *gc.C) {
	client := FakeHTTPClient{Data: bytes.NewBufferString("raw data")}
	resp, err := rawrpc.Send(&client, nil, nil)
	data, _ := ioutil.ReadAll(resp.Body)

	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "raw data")
}

func (s *rawrpcSuite) TestSendRequestSendFailed(c *gc.C) {
	client := FakeHTTPClient{Err: fmt.Errorf("failed!")}
	_, err := rawrpc.Send(&client, nil, nil)

	c.Assert(err, gc.ErrorMatches, "could not send raw request: .*")
}

func (s *rawrpcSuite) TestSendMethodNotSupported(c *gc.C) {
	client := FakeHTTPClient{Code: http.StatusMethodNotAllowed}
	_, err := rawrpc.Send(&client, nil, nil)

	c.Assert(err, gc.ErrorMatches, "method not supported by API server")
}

func (s *rawrpcSuite) TestSendResultError(c *gc.C) {
	client := FakeHTTPClient{
		Data: bytes.NewBufferString(`{"Error": "failed!"}`),
		Code: http.StatusInternalServerError,
	}
	var result FakeResult
	_, err := rawrpc.Send(&client, nil, &result)

	c.Assert(err, gc.ErrorMatches, "request failed on server: .*")
}

func (s *rawrpcSuite) TestSendBadStatusCode(c *gc.C) {
	client := FakeHTTPClient{
		Data: bytes.NewBufferString(`{}`),
		Code: http.StatusInternalServerError,
	}
	var result FakeResult
	_, err := rawrpc.Send(&client, nil, &result)

	c.Assert(err.Error(), gc.Equals, "request failed on server (500)")
}
