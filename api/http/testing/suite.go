// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiserverhttp "github.com/juju/juju/apiserver/http"
	apihttptesting "github.com/juju/juju/apiserver/http/testing"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

// HTTPSuite provides basic testing capability for API HTTP tests.
type HTTPSuite struct {
	testing.BaseSuite

	// Fake is the fake HTTP client used in tests.
	Fake *FakeHTTPClient

	// Hostname is the API server's hostname.
	Hostname string

	// Username is the username to use for API connections.
	Username string

	// Password is the password to use for API connections.
	Password string
}

func (s *HTTPSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Fake = NewFakeHTTPClient()
	s.Hostname = "localhost"
	s.Username = dummy.AdminUserTag().String()
	s.Password = jujutesting.AdminSecret
}

// CheckRequest verifies that the HTTP request matches the args
// as an API request should.  We only check API-related request fields.
func (s *HTTPSuite) CheckRequest(c *gc.C, req *http.Request, method, path string) {
	apihttptesting.CheckRequest(c, req, method, s.Username, s.Password, s.Hostname, path)
}

// APIMethodSuite provides testing functionality for specific API methods.
type APIMethodSuite struct {
	HTTPSuite

	// HTTPMethod is the HTTP method to use for the suite.
	HTTPMethod string

	// Name is the name of the API method.
	Name string
}

// CheckRequest verifies that the HTTP request matches the args
// as an API request should.  We only check API-related request fields.
func (s *APIMethodSuite) CheckRequest(c *gc.C, req *http.Request) {
	s.HTTPSuite.CheckRequest(c, req, s.HTTPMethod, s.Name)
}

// APIHTTPClientSuite wraps a fake API HTTP client (see api/http.go).
// It provides methods for setting the response the client will return.
type APIHTTPClientSuite struct {
	testing.BaseSuite

	// FakeClient is the fake API HTTP Client that may be used in testing.
	FakeClient FakeClient
}

// SetResponse sets the HTTP response on the fake client using the
// provided values. The data is set as the body of the response.
func (s *APIHTTPClientSuite) SetResponse(c *gc.C, status int, data []byte, ctype string) {
	resp := http.Response{
		StatusCode: status,
		Header:     make(http.Header),
	}

	resp.Header.Set("Content-Type", ctype)
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(data))

	s.FakeClient.Response = &resp
}

// SetJSONSuccess sets a success response on the fake client. The
// provided result is JSON-encoded and set as the body of the response.
// The content-type is thus application/json. A status code of
// http.StatusOK (200) is used.
func (s *APIHTTPClientSuite) SetJSONSuccess(c *gc.C, result interface{}) {
	status := http.StatusOK
	data, err := json.Marshal(result)
	c.Assert(err, jc.ErrorIsNil)

	s.SetResponse(c, status, data, apiserverhttp.CTypeJSON)
}

// SetFailure sets a failure response on the fake client. The provided
// message is packed into an apiserver/params.Error. That error is then
// set as the body of the response. The content-type is thus
// application/json.
func (s *APIHTTPClientSuite) SetFailure(c *gc.C, msg string, status int) {
	failure := params.Error{
		Message: msg,
	}
	data, err := json.Marshal(&failure)
	c.Assert(err, jc.ErrorIsNil)

	s.SetResponse(c, status, data, apiserverhttp.CTypeJSON)
}

// SetError sets an error response on the fake client. A content-type
// of application/octet-stream is used. The provided message is set as
// the body of the response. Any status code less than 0 is replaced
// with http.StatusInternalServerError (500).
func (s *APIHTTPClientSuite) SetError(c *gc.C, msg string, status int) {
	if status < 0 {
		status = http.StatusInternalServerError
	}

	data := []byte(msg)
	s.SetResponse(c, status, data, apiserverhttp.CTypeRaw)
}
