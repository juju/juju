// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"net/http"

	gc "gopkg.in/check.v1"
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
	resp := NewHTTPResponse()
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
