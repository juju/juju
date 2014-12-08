// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"
	"net/http"

	gc "gopkg.in/check.v1"

	apihttptesting "github.com/juju/juju/apiserver/http/testing"
)

// FakeHTTPClient is used in testing in place of an actual http.Client.
type FakeHTTPClient struct {
	// Error is the error that will be returned for any calls.
	Error error

	// Response is the response returned from calls.
	Response *http.Response

	// Calls is the record of which methods were called.
	Calls []string

	// RequestArg is the request that was passed to a call.
	RequestArg *http.Request
}

// NewFakeHTTPClient returns a fake with Response set to an OK status,
// no headers, and no body.
func NewFakeHTTPClient() *FakeHTTPClient {
	resp := apihttptesting.NewHTTPResponse()
	fake := FakeHTTPClient{
		Response: &resp.Response,
	}
	return &fake
}

// CheckCalled checks that the Do was called once with the request and
// returned the correct value.
func (f *FakeHTTPClient) CheckCalled(c *gc.C, req *http.Request, resp *http.Response) {
	c.Check(f.Calls, gc.DeepEquals, []string{"Do"})
	c.Check(f.RequestArg, gc.Equals, req)
	c.Check(resp, gc.Equals, f.Response)
}

// Do fakes the behavior of http.Client.Do().
func (f *FakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	f.Calls = append(f.Calls, "Do")
	f.RequestArg = req
	return f.Response, f.Error
}

type FakeClient struct {
	calls       []string
	pathArg     string
	argsArg     interface{}
	attachedArg io.Reader
	metaArg     interface{}
	nameArg     string

	// Error is the error that will be returned for any calls.
	Error error
	// Request is the request returned from calls.
	Request *http.Request
	// Response is the response returned from calls.
	Response *http.Response
}

func (f *FakeClient) SendHTTPRequest(path string, args interface{}) (*http.Request, *http.Response, error) {
	f.calls = append(f.calls, "SendHTTPRequest")
	f.pathArg = path
	f.argsArg = args
	return f.Request, f.Response, f.Error
}

func (f *FakeClient) SendHTTPRequestReader(path string, attached io.Reader, meta interface{}, name string) (*http.Request, *http.Response, error) {
	f.calls = append(f.calls, "SendHTTPRequestReader")
	f.pathArg = path
	f.attachedArg = attached
	f.metaArg = meta
	f.nameArg = name
	return f.Request, f.Response, f.Error
}

// CheckCalled checks that the fake was called properly.
func (f *FakeClient) CheckCalled(c *gc.C, path string, args interface{}, calls ...string) {
	c.Check(f.calls, gc.DeepEquals, calls)
	c.Check(f.pathArg, gc.Equals, path)
	c.Check(f.argsArg, gc.Equals, args)
}

// CheckCalledReader checks that the fake was called properly.
func (f *FakeClient) CheckCalledReader(c *gc.C, path string, attached io.Reader, meta interface{}, name string, calls ...string) {
	c.Check(f.calls, gc.DeepEquals, calls)
	c.Check(f.pathArg, gc.Equals, path)
	c.Check(f.attachedArg, gc.Equals, attached)
	c.Check(f.metaArg, gc.DeepEquals, meta)
	c.Check(f.nameArg, gc.Equals, name)
}
