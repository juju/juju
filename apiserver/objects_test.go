// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

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

func (s *baseObjectsSuite) uploadRequest(c *gc.C, url, contentType, curl string, content io.Reader) *http.Response {
	return sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "PUT",
		URL:         url,
		ContentType: contentType,
		Body:        content,
		ExtraHeaders: map[string]string{
			"Juju-Curl": curl,
		},
	})
}

func (s *baseObjectsSuite) objectsCharmsURL(charmurl string) *url.URL {
	return s.URL(fmt.Sprintf("/model-%s/charms/%s", s.ControllerModelUUID(), charmurl), nil)
}

func (s *baseObjectsSuite) objectsCharmsURI(charmurl string) string {
	return s.objectsCharmsURL(charmurl).String()
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

func (s *getCharmObjectSuite) TestGetReturnsNotYetAvailableForPendingCharms(c *gc.C) {
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

func (s *getCharmObjectSuite) TestGetReturnsMatchingContents(c *gc.C) {
	chArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	f, err := os.Open(chArchive.Path)
	c.Assert(err, jc.ErrorIsNil)
	_ = s.uploadRequest(c, s.objectsCharmsURI("dummy-"+getCharmHash(c, f)), "application/zip", "local:quantal/dummy-1", &fileReader{path: chArchive.Path})

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

type putCharmObjectSuite struct {
	baseObjectsSuite
}

var _ = gc.Suite(&putCharmObjectSuite{})

func (s *putCharmObjectSuite) SetUpSuite(c *gc.C) {
	s.baseObjectsSuite.SetUpSuite(c)
	s.baseObjectsSuite.method = "PUT"
	s.baseObjectsSuite.contentType = "application/zip"
}

func (s *putCharmObjectSuite) assertUploadResponse(c *gc.C, resp *http.Response, expCharmURL string) {
	charmResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(charmResponse.Error, gc.Equals, "")
	c.Check(charmResponse.CharmURL, gc.Equals, expCharmURL)
}

func (s *putCharmObjectSuite) TestUploadFailsWithInvalidZip(c *gc.C) {
	empty := strings.NewReader("")

	// Pretend we upload a zip by setting the Content-Type, so we can
	// check the error at extraction time later.
	resp := s.uploadRequest(c, s.objectsCharmsURI("somecharm-"+getCharmHash(c, empty)), "application/zip", "local:somecharm", empty)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*cannot open charm archive: zip: not a valid zip file$")

	// Now try with the default Content-Type.
	resp = s.uploadRequest(c, s.objectsCharmsURI("somecharm-"+getCharmHash(c, empty)), "application/octet-stream", "local:somecharm", empty)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*expected Content-Type: application/zip, got: application/octet-stream$")
}

func (s *putCharmObjectSuite) TestUploadBumpsRevision(c *gc.C) {
	// Add the dummy charm with revision 1.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision())
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-storage-path",
		SHA256:      "dummy-1-sha256",
	}
	_, err := s.ControllerModel(c).State().AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// Now try uploading the same revision and verify it gets bumped,
	// and the BundleSha256 is calculated.
	f, err := os.Open(ch.Path)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	resp := s.uploadRequest(c, s.objectsCharmsURI("dummy-"+getCharmHash(c, f)), "application/zip", "local:quantal/dummy", f)
	expectedURL := "local:quantal/dummy-2"
	s.assertUploadResponse(c, resp, expectedURL)
	sch, err := s.ControllerModel(c).State().Charm(expectedURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.Equals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 2)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	// No more checks for the hash here, because it is
	// verified in TestUploadRespectsLocalRevision.
	c.Assert(sch.BundleSha256(), gc.Not(gc.Equals), "")
}

func getCharmHash(c *gc.C, stream io.ReadSeeker) string {
	hash := sha256.New()
	_, err := io.Copy(hash, stream)
	c.Assert(err, jc.ErrorIsNil)
	_, err = stream.Seek(0, os.SEEK_SET)
	c.Assert(err, jc.ErrorIsNil)
	return hex.EncodeToString(hash.Sum(nil))[0:7]
}
