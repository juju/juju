// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net/http"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apihttp "github.com/juju/juju/api/http"
	apihttptesting "github.com/juju/juju/api/http/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type httpSuite struct {
	apihttptesting.HTTPSuite
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&httpSuite{})

func (s *httpSuite) SetUpSuite(c *gc.C) {
	s.HTTPSuite.SetUpSuite(c)
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *httpSuite) TearDownSuite(c *gc.C) {
	s.HTTPSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *httpSuite) SetUpTest(c *gc.C) {
	s.HTTPSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)

	// This determines the client used in SendHTTPRequest().
	s.PatchValue(api.NewHTTPClient,
		func(api.Connection) apihttp.HTTPClient {
			return s.Fake
		},
	)
}

func (s *httpSuite) TearDownTest(c *gc.C) {
	s.HTTPSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *httpSuite) TestNewHTTPRequestSuccess(c *gc.C) {
	req, err := s.APIState.NewHTTPRequest("GET", "somefacade")
	c.Assert(err, jc.ErrorIsNil)

	s.CheckRequest(c, req, "GET", "somefacade")
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
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resp.StatusCode, gc.Equals, http.StatusNotFound)
}

func (s *httpSuite) TestSendHTTPRequestSuccess(c *gc.C) {
	req, resp, err := s.APIState.SendHTTPRequest("somefacade", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.Fake.CheckCalled(c, req, resp)
}
