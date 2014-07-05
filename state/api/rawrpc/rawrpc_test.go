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
// Do() tests

func (s *rawrpcSuite) TestDoValidNoData(c *gc.C) {
	client := FakeHTTPClient{}
	resp, err := rawrpc.Do(&client, nil)
	data, _ := ioutil.ReadAll(resp.Body)

	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "")
}

func (s *rawrpcSuite) TestDoValidData(c *gc.C) {
	client := FakeHTTPClient{Data: bytes.NewBufferString("raw data")}
	resp, err := rawrpc.Do(&client, nil)
	data, _ := ioutil.ReadAll(resp.Body)

	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "raw data")
}

func (s *rawrpcSuite) TestDoRequestSendFailed(c *gc.C) {
	client := FakeHTTPClient{Err: fmt.Errorf("failed!")}
	_, err := rawrpc.Do(&client, nil)

	c.Assert(err, gc.ErrorMatches, "could not send raw request: .*")
}

func (s *rawrpcSuite) TestDoMethodNotSupported(c *gc.C) {
	client := FakeHTTPClient{Code: http.StatusMethodNotAllowed}
	_, err := rawrpc.Do(&client, nil)

	c.Assert(err, gc.ErrorMatches, "API method not supported by server")
}

// tests for method failures returned by the API server

func (s *rawrpcSuite) TestDoUnreadableErrorData(c *gc.C) {
	client := FakeHTTPClient{
		Data: InvalidData("invalid!"),
		Code: http.StatusInternalServerError,
	}
	_, err := rawrpc.Do(&client, nil)

	c.Assert(err, gc.ErrorMatches, "could not unpack error response: .*")
}

func (s *rawrpcSuite) TestDoBadErrorData(c *gc.C) {
	client := FakeHTTPClient{
		Data: bytes.NewBufferString("not valid JSON"),
		Code: http.StatusInternalServerError,
	}
	_, err := rawrpc.Do(&client, nil)

	c.Assert(err, gc.ErrorMatches, "could not unpack error response: .*")
}

func (s *rawrpcSuite) TestDoFailedRemotely(c *gc.C) {
	client := FakeHTTPClient{
		Data: bytes.NewBufferString(`{"Message": "failed!"}`),
		Code: http.StatusInternalServerError,
	}
	_, err := rawrpc.Do(&client, nil)

	c.Assert(err, gc.ErrorMatches, "failed!")
}

func (s *rawrpcSuite) TestDoBadStatusCode(c *gc.C) {
	client := FakeHTTPClient{
		Data: bytes.NewBufferString(`{}`),
		Code: http.StatusInternalServerError,
	}
	_, err := rawrpc.Do(&client, nil)

	c.Assert(err.Error(), gc.Equals, "")
}
