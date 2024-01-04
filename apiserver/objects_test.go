// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
)

type objectsSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&objectsSuite{})

func (s *objectsSuite) SetUpSuite(c *gc.C) {
	s.apiserverBaseSuite.SetUpSuite(c)
}

func (s *objectsSuite) objectsCharmsURL(charmurl string) *url.URL {
	return s.URL(fmt.Sprintf("/model-%s/charms/%s", s.State.ModelUUID(), charmurl), nil)
}

func (s *objectsSuite) objectsCharmsURI(charmurl string) string {
	return s.objectsCharmsURL(charmurl).String()
}

func (s *objectsSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.CharmsResponse {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var charmResponse params.CharmsResponse
	err := json.Unmarshal(body, &charmResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	return charmResponse
}

func (s *objectsSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	charmResponse := s.assertResponse(c, resp, expCode)
	c.Check(charmResponse.Error, gc.Matches, expError)
}

func (s *objectsSuite) TestObjectsCharmsServedSecurely(c *gc.C) {
	url := s.objectsCharmsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *objectsSuite) TestGETRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsCharmsURI("somecharm-abcd0123")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *objectsSuite) TestOnlyMethodGET(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.objectsCharmsURI("somecharm-abcd0123")})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "Method Not Allowed\n")
}

func (s *objectsSuite) TestGetFailsWithInvalidObjectSha256(c *gc.C) {
	uri := s.objectsCharmsURI("invalidsha256")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`.*wrong charms object path "invalidsha256"$`,
	)
}

func (s *objectsSuite) TestInvalidBucket(c *gc.C) {
	wrongURL := s.URL("modelwrongbucket/charms/somecharm-abcd0123", nil)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusNotFound, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "404 page not found\n")
}

func (s *objectsSuite) TestInvalidModel(c *gc.C) {
	wrongURL := s.URL("model-wrongbucket/charms/somecharm-abcd0123", nil)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "invalid model UUID \"wrongbucket\"\n")
}

func (s *objectsSuite) TestInvalidObject(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsCharmsURI("invalidcharm")})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "application/json")
	c.Assert(string(body), gc.Equals, "{\"error\":\"cannot retrieve charm: wrong charms object path \\\"invalidcharm\\\"\",\"error-code\":\"bad request\"}")
}
