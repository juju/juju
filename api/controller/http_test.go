// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"net"
	"net/http"
	"net/url"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/testing"
)

// newHTTPFixture creates and returns an HTTP fixture to be used in order to
// mock controller HTTP requests to the given controller address.
// Use it like in the following example:
//   fix := newHTTPFixture("/my/controller/path", func(w http.ResponseWriter, req *http.Request) {
//       // Simulate what's returned by the server.
//   })
//   stub := fix.run(c, func(ac base.APICallCloser) {
//       // Do something with the API caller.
//   })
// At this point the stub, if the handler has been called, includes one call
// with the HTTP method requested while running the test function.
func newHTTPFixture(address string, handle func(http.ResponseWriter, *http.Request)) *httpFixture {
	return &httpFixture{
		address: address,
		handle:  handle,
	}
}

// httpFixture is used to mock controller HTTP API calls.
type httpFixture struct {
	address string
	handle  func(http.ResponseWriter, *http.Request)
}

// run sets up the fixture and run the given test. See newHTTPFixture for an
// example of how to use this.
func (f *httpFixture) run(c *gc.C, test func(base.APICallCloser)) *testing.Stub {
	stub := &testing.Stub{}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)
	defer lis.Close()
	mux := http.NewServeMux()
	mux.HandleFunc(f.address, func(w http.ResponseWriter, r *http.Request) {
		stub.AddCall(r.Method)
		f.handle(w, r)
	})
	go func() {
		http.Serve(lis, mux)
	}()
	test(&httpAPICallCloser{
		url: &url.URL{
			Scheme: "http",
			Host:   lis.Addr().String(),
		},
	})
	return stub
}

var _ base.APICallCloser = (*httpAPICallCloser)(nil)

// httpAPICallCloser implements base.APICallCloser.
type httpAPICallCloser struct {
	base.APICallCloser
	url *url.URL
}

// ModelTag implements base.APICallCloser.
func (*httpAPICallCloser) ModelTag() (names.ModelTag, bool) {
	return coretesting.ModelTag, true
}

// BestFacadeVersion implements base.APICallCloser.
func (*httpAPICallCloser) BestFacadeVersion(facade string) int {
	return 42
}

// BestFacadeVersion implements base.APICallCloser.
func (*httpAPICallCloser) Context() context.Context {
	return context.Background()
}

// HTTPClient implements base.APICallCloser. The returned HTTP client can be
// used to send requests to the testing server set up in httpFixture.run().
func (ac *httpAPICallCloser) HTTPClient() (*httprequest.Client, error) {
	return &httprequest.Client{
		BaseURL: ac.url.String(),
	}, nil
}
