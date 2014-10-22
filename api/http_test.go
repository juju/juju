// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bytes"
	"encoding/base64"
	"io/ioutil"
	"net/http"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
)

type fakeHTTPClient struct {
	calls []string

	response *http.Response
	err      error

	reqArg *http.Request
}

func (d *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	d.calls = append(d.calls, "Do")
	d.reqArg = req
	return d.response, d.err
}

type httpSuite struct {
	jujutesting.JujuConnSuite
	fake *fakeHTTPClient
}

var _ = gc.Suite(&httpSuite{})

func (s *httpSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	resp := http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       ioutil.NopCloser(&bytes.Buffer{}),
	}

	fake := fakeHTTPClient{
		response: &resp,
	}
	s.fake = &fake

	s.PatchValue(api.NewHTTPClient,
		func(*api.State) api.HTTPClient {
			return s.fake
		},
	)
}

func (s *httpSuite) checkRequest(c *gc.C, req *base.HTTPRequest, method, path string) {
	// Only check API-related request fields.

	c.Check(req.Method, gc.Equals, method)

	url := `https://localhost:\d+/environment/[-0-9a-f]+/` + path
	c.Check(req.URL.String(), gc.Matches, url)

	c.Assert(req.Header, gc.HasLen, 1)
	username := dummy.AdminUserTag().String()
	password := jujutesting.AdminSecret
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	c.Check(req.Header.Get("Authorization"), gc.Equals, "Basic "+auth)
}

func (s *httpSuite) TestNewHTTPRequestSuccess(c *gc.C) {
	req, err := s.APIState.NewHTTPRequest("GET", "somefacade")
	c.Assert(err, gc.IsNil)

	s.checkRequest(c, req, "GET", "somefacade")
}

func (s *httpSuite) TestNewHTTPClientCorrectTransport(c *gc.C) {
	apiHTTPClient := s.APIState.NewHTTPClient()

	c.Assert(apiHTTPClient, gc.FitsTypeOf, (*http.Client)(nil))
	httpClient := apiHTTPClient.(*http.Client)

	c.Assert(httpClient.Transport, gc.NotNil)
	c.Assert(httpClient.Transport, gc.FitsTypeOf, (*http.Transport)(nil))
	config := httpClient.Transport.(*http.Transport).TLSClientConfig

	c.Check(config.RootCAs, gc.NotNil)
}

func (s *httpSuite) TestNewHTTPClientValidatesCert(c *gc.C) {
	req, err := s.APIState.NewHTTPRequest("GET", "somefacade")
	httpClient := s.APIState.NewHTTPClient()
	resp, err := httpClient.Do(&req.Request)
	c.Assert(err, gc.IsNil)

	c.Check(resp.StatusCode, gc.Equals, http.StatusNotFound)
}

func (s *httpSuite) TestSendHTTPRequestSuccess(c *gc.C) {
	req, err := s.APIState.NewHTTPRequest("GET", "somefacade")
	c.Assert(err, gc.IsNil)
	resp, err := s.APIState.SendHTTPRequest(req)
	c.Assert(err, gc.IsNil)

	c.Check(s.fake.calls, gc.DeepEquals, []string{"Do"})
	c.Check(s.fake.reqArg, gc.Equals, &req.Request)
	c.Check(resp, gc.Equals, s.fake.response)
}
