// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"encoding/base64"
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

type baseObjectsSuite struct {
	jujutesting.ApiServerSuite
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

func (s *baseObjectsSuite) uploadRequest(c *gc.C, url, contentType string, content io.Reader) *http.Response {
	return sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url,
		ContentType: contentType,
		Body:        content,
	})
}

// TODO(jack-w-shaw) Once we have implemented PutObject S3 endpoint, drop these next three
// methods and use PutObject instead
func (s *baseObjectsSuite) charmsURL(query string) *url.URL {
	url := s.URL(fmt.Sprintf("/model/%s/charms", s.ControllerModelUUID()), nil)
	url.RawQuery = query
	return url
}

func (s *baseObjectsSuite) charmsURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.charmsURL(query).String()
}

type charmObjectsSuite struct {
	baseObjectsSuite
}

var _ = gc.Suite(&charmObjectsSuite{})

func (s *charmObjectsSuite) TestObjectsCharmsServedSecurely(c *gc.C) {
	url := s.objectsCharmsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *charmObjectsSuite) TestGETRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsCharmsURI("somecharm-abcd0123")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *charmObjectsSuite) TestOnlyMethodGET(c *gc.C) {
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.objectsCharmsURI("somecharm-abcd0123")})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "Method Not Allowed\n")
}

func (s *charmObjectsSuite) TestGetFailsWithInvalidObjectSha256(c *gc.C) {
	uri := s.objectsCharmsURI("invalidsha256")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`.*wrong charms object path "invalidsha256"$`,
	)
}

func (s *charmObjectsSuite) TestInvalidBucket(c *gc.C) {
	wrongURL := s.URL("modelwrongbucket/charms/somecharm-abcd0123", nil)
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "invalid bucket format \"modelwrongbucket\"\n")
}

func (s *charmObjectsSuite) TestInvalidModel(c *gc.C) {
	wrongURL := s.URL("model-wrongbucket/charms/somecharm-abcd0123", nil)
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "invalid model UUID \"wrongbucket\"\n")
}

func (s *charmObjectsSuite) TestInvalidObject(c *gc.C) {
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsCharmsURI("invalidcharm")})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "application/json")
	c.Assert(string(body), gc.Equals, `{"error":"cannot retrieve charm: wrong charms object path \"invalidcharm\"","error-code":"bad request"}`)
}

func (s *charmObjectsSuite) TestGetReturnsNotYetAvailableForPendingCharms(c *gc.C) {
	fakeSHA256 := "123456789abcde123456789abcde123456789abcde123456789abcde12345678"
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

func (s *charmObjectsSuite) TestGetReturnsMatchingContents(c *gc.C) {
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

func (s *charmObjectsSuite) objectsCharmsURL(charmurl string) *url.URL {
	return s.URL(fmt.Sprintf("/model-%s/charms/%s", s.ControllerModelUUID(), charmurl), nil)
}

func (s *charmObjectsSuite) objectsCharmsURI(charmurl string) string {
	return s.objectsCharmsURL(charmurl).String()
}

type objectsSuite struct {
	baseObjectsSuite
}

var _ = gc.Suite(&objectsSuite{})

func (s *objectsSuite) SetUpSuite(c *gc.C) {
	s.ApiServerSuite.SetUpSuite(c)
}

func (s *objectsSuite) TestObjectsCharmsServedSecurely(c *gc.C) {
	url := s.objectsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *objectsSuite) TestGETRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsURL("somecharm-abcd0123").String()})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *objectsSuite) TestInvalidModel(c *gc.C) {
	wrongURL := s.URL("model-wrongbucket/charms/somecharm-abcd0123", nil)
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: wrongURL.String()})
	body := apitesting.AssertResponse(c, resp, http.StatusBadRequest, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "invalid model UUID \"wrongbucket\"\n")
}

func (s *objectsSuite) TestOnlyMethodGET(c *gc.C) {
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.objectsURL("somecharm-abcd0123").String()})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "Method Not Allowed\n")
}

func (s *objectsSuite) TestGetFailsWithEmptyObjectID(c *gc.C) {
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.objectsURL("").String()})
	apitesting.AssertResponse(c, resp, http.StatusNotFound, "text/plain; charset=utf-8")
}

func (s *objectsSuite) TestGetFailsWithInvalidObjectID(c *gc.C) {
	url := s.URL(fmt.Sprintf("/model-%s/objects/%s", s.ControllerModelUUID(), "blah="), nil).String()
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`.*cannot decode object id$`,
	)
}

func (s *objectsSuite) TestGetReturnsNotFound(c *gc.C) {
	// Notice that the returned content-type is application/json when it's not
	// found.
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsURL("base").String()})
	apitesting.AssertResponse(c, resp, http.StatusNotFound, "application/json")
}

func (s *objectsSuite) TestGetReturnsMatchingContents(c *gc.C) {
	chArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	// use legacy upload endpoint as PutObject is not yet implemented
	_ = s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: chArchive.Path})

	// Get the charm back out so we can see the storage path.
	ch, err := s.ControllerModel(c).State().Charm("local:quantal/dummy-1")
	c.Assert(err, jc.ErrorIsNil)

	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.objectsURL(ch.StoragePath()).String()})
	body := apitesting.AssertResponse(c, resp, http.StatusOK, "application/zip")
	archiveBytes, err := os.ReadFile(chArchive.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bytes.Equal(body, archiveBytes), jc.IsTrue)
}

func (s *objectsSuite) objectsURL(path string) *url.URL {
	objectID := base64.URLEncoding.EncodeToString([]byte(path))
	return s.URL(fmt.Sprintf("/model-%s/objects/%s", s.ControllerModelUUID(), objectID), nil)
}
