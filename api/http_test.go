// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net/http"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apihttp "github.com/juju/juju/api/http"
	apihttptesting "github.com/juju/juju/api/http/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
)

type httpSuite struct {
	apihttptesting.BaseSuite
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&httpSuite{})

func (s *httpSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)

	// This determines the client used in SendHTTPRequest().
	s.PatchValue(api.NewHTTPClient,
		func(*api.State) apihttp.HTTPClient {
			return s.Fake
		},
	)
}

func (s *httpSuite) checkRequest(c *gc.C, req *http.Request, method, path string) {
	username := dummy.AdminUserTag().String()
	password := jujutesting.AdminSecret
	hostname := "localhost"
	s.CheckRequest(c, req, method, username, password, hostname, path)
}

func (s *httpSuite) TestNewHTTPRequestSuccess(c *gc.C) {
	req, err := s.APIState.NewHTTPRequest("GET", "somefacade")
	c.Assert(err, gc.IsNil)

	s.checkRequest(c, req, "GET", "somefacade")
}

func (s *httpSuite) TestNewHTTPClientCorrectTransport(c *gc.C) {
	httpClient := s.APIState.NewHTTPClient()

	c.Assert(httpClient.Transport, gc.NotNil)
	c.Assert(httpClient.Transport, gc.FitsTypeOf, (*http.Transport)(nil))
	config := httpClient.Transport.(*http.Transport).TLSClientConfig

	c.Check(config.RootCAs, gc.NotNil)
}

func (s *httpSuite) TestNewHTTPClientValidatesCert(c *gc.C) {
	req, err := s.APIState.NewHTTPRequest("GET", "somefacade")
	httpClient := s.APIState.NewHTTPClient()
	resp, err := httpClient.Do(req)
	c.Assert(err, gc.IsNil)

	c.Check(resp.StatusCode, gc.Equals, http.StatusNotFound)
}

func (s *httpSuite) TestSendHTTPRequestSuccess(c *gc.C) {
	req, resp, err := s.APIState.SendHTTPRequest("GET", "somefacade", nil)
	c.Assert(err, gc.IsNil)

	s.Fake.CheckCalled(c, req, resp)
}
