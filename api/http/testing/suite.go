// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"net/http"

	gc "gopkg.in/check.v1"

	apihttptesting "github.com/juju/juju/apiserver/http/testing"
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
