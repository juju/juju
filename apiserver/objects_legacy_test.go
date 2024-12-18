// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type baseObjectsSuite struct {
	jujutesting.ApiServerSuite

	method      string
	contentType string
}

func (s *baseObjectsSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.CharmsResponse {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var charmResponse params.CharmsResponse
	err := json.Unmarshal(body, &charmResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	return charmResponse
}

func (s *baseObjectsSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	charmResponse := s.assertResponse(c, resp, expCode)
	c.Check(charmResponse.Error, gc.Matches, expError)
}

func (s *baseObjectsSuite) objectsCharmsURL(charmRef string) *url.URL {
	return s.URL(fmt.Sprintf("/model-%s/charms/%s", s.ControllerModelUUID(), charmRef), nil)
}

func (s *baseObjectsSuite) objectsCharmsURI(charmRef string) string {
	return s.objectsCharmsURL(charmRef).String()
}

func (s *baseObjectsSuite) TestObjectsCharmsServedSecurely(c *gc.C) {
	url := s.objectsCharmsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *baseObjectsSuite) TestRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, URL: s.objectsCharmsURI("somecharm-abcd0123")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *baseObjectsSuite) TestGetFailsWithInvalidObjectSha256(c *gc.C) {
	uri := s.objectsCharmsURI("invalidsha256")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, ContentType: s.contentType, URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`.*"invalidsha256" is not a valid charm object path$`,
	)
}

func (s *baseObjectsSuite) TestInvalidBucket(c *gc.C) {
	wrongURL := s.URL("modelwrongbucket/charms/somecharm-abcd0123", nil)
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusNotFound, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "404 page not found\n")
}

func (s *baseObjectsSuite) TestInvalidModel(c *gc.C) {
	wrongURL := s.URL("model-wrongbucket/charms/somecharm-abcd0123", nil)
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "invalid model UUID \"wrongbucket\"\n")
}

func (s *baseObjectsSuite) TestInvalidObject(c *gc.C) {
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: s.method, ContentType: s.contentType, URL: s.objectsCharmsURI("invalidcharm")})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "application/json")
	c.Assert(string(body), gc.Matches, `{"error":".*\\"invalidcharm\\" is not a valid charm object path","error-code":"bad request"}$`)
}

type getCharmObjectSuite struct {
	baseObjectsSuite
}

var _ = gc.Suite(&getCharmObjectSuite{})

func (s *getCharmObjectSuite) SetUpTest(c *gc.C) {
	s.baseObjectsSuite.SetUpTest(c)
	s.method = "GET"
}

type putCharmObjectSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&putCharmObjectSuite{})

func (s *putCharmObjectSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- PUT (charm into object store) requires auth.
- Upload fails when zip is invalid. 
- Prevent the upload of charmhub charms. 
- Revision should be bumped when the same charm is uploaded again.
- Charm version should be correct after upload.
- Charm revision should be the same as the one in disk after upload.
- CharmHub charm should be correctly uploaded during migration.
- Uploading a charm with an unknown schema on the charm url should fail.
- Uploading a non-local charm with revision override should not fail.
- Charm migration should work.
- Charm upload should fail if migration mode is not "importing".
- User should be authorized to upload a charm, even during migration.
`)
}
