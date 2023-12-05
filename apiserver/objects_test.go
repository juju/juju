// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

type objectsSuite struct {
	jujutesting.ApiServerSuite
}

var _ = gc.Suite(&objectsSuite{})

func (s *objectsSuite) SetUpSuite(c *gc.C) {
	s.ApiServerSuite.SetUpSuite(c)
}

func (s *objectsSuite) objectsCharmsURL(charmurl string) *url.URL {
	return s.URL(fmt.Sprintf("/model-%s/charms/%s", s.ControllerModelUUID(), charmurl), nil)
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
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.objectsCharmsURI("somecharm-abcd0123")})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "Method Not Allowed\n")
}

func (s *objectsSuite) TestGetFailsWithInvalidObjectSha256(c *gc.C) {
	uri := s.objectsCharmsURI("invalidsha256")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`.*wrong charms object path "invalidsha256"$`,
	)
}

func (s *objectsSuite) TestInvalidBucket(c *gc.C) {
	wrongURL := s.URL("modelwrongbucket/charms/somecharm-abcd0123", nil)
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "invalid bucket format \"modelwrongbucket\"\n")
}

func (s *objectsSuite) TestInvalidModel(c *gc.C) {
	wrongURL := s.URL("model-wrongbucket/charms/somecharm-abcd0123", nil)
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "invalid model UUID \"wrongbucket\"\n")
}

func (s *objectsSuite) TestInvalidObject(c *gc.C) {
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsCharmsURI("invalidcharm")})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "application/json")
	c.Assert(string(body), gc.Equals, `{"error":"cannot retrieve charm: wrong charms object path \"invalidcharm\"","error-code":"bad request"}`)
}

var fakeSHA256 = "123456789abcde123456789abcde123456789abcde123456789abcde12345678"

func (s *objectsSuite) TestGetReturnsNotYetAvailableForPendingCharms(c *gc.C) {
	// Add a charm in pending mode.
	chInfo := state.CharmInfo{
		ID:          "ch:focal/dummy-1",
		Charm:       testcharms.Repo.CharmArchive(c.MkDir(), "dummy"),
		StoragePath: "", // indicates that we don't have the data in the blobstore yet.
		SHA256:      fakeSHA256,
		Version:     "42",
	}
	_, err := s.ControllerModel(c).State().AddCharmMetadata(chInfo)
	c.Assert(err, jc.ErrorIsNil)

	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsCharmsURI("dummy-" + fakeSHA256)})
	body := apitesting.AssertResponse(c, resp, http.StatusConflict, "application/json")
	c.Assert(string(body), gc.Equals, `{"error":"cannot retrieve charm: ch:focal/dummy-1","error-code":"not yet available; try again later"}`)
}

// TODO(jack-w-shaw) Once we have implemented PutObject S3 endpoint, drop these next three
// methods and use PutObject instead
func (s *objectsSuite) charmsURL(query string) *url.URL {
	url := s.URL(fmt.Sprintf("/model/%s/charms", s.ControllerModelUUID()), nil)
	url.RawQuery = query
	return url
}

func (s *objectsSuite) charmsURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.charmsURL(query).String()
}

func (s *objectsSuite) uploadRequest(c *gc.C, url, contentType string, content io.Reader) *http.Response {
	return sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url,
		ContentType: contentType,
		Body:        content,
	})
}

func (s *objectsSuite) TestGetReturnsMatchingContents(c *gc.C) {
	chArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	// use legacy upload endpoint as PutObject is not yet implemented
	_ = s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: chArchive.Path})

	// get uploaded charm's SHA256 for GET request
	ch, err := s.ControllerModel(c).State().Charm("local:quantal/dummy-1")
	c.Assert(err, jc.ErrorIsNil)
	sha256 := ch.BundleSha256()

	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsCharmsURI("dummy-" + sha256)})
	body := apitesting.AssertResponse(c, resp, http.StatusOK, "application/zip")
	archiveBytes, err := os.ReadFile(chArchive.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bytes.Equal(body, archiveBytes), jc.IsTrue)
}
