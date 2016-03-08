// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testcharms"
)

// charmsCommonSuite wraps authHttpSuite and adds
// some helper methods suitable for working with the
// charms endpoint.
type charmsCommonSuite struct {
	authHttpSuite
}

func (s *charmsCommonSuite) charmsURL(c *gc.C, query string) *url.URL {
	uri := s.baseURL(c)
	if s.modelUUID == "" {
		uri.Path = "/charms"
	} else {
		uri.Path = fmt.Sprintf("/model/%s/charms", s.modelUUID)
	}
	uri.RawQuery = query
	return uri
}

func (s *charmsCommonSuite) charmsURI(c *gc.C, query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.charmsURL(c, query).String()
}

func (s *charmsCommonSuite) assertUploadResponse(c *gc.C, resp *http.Response, expCharmURL string) {
	charmResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(charmResponse.Error, gc.Equals, "")
	c.Check(charmResponse.CharmURL, gc.Equals, expCharmURL)
}

func (s *charmsCommonSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := assertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *charmsCommonSuite) assertGetFileListResponse(c *gc.C, resp *http.Response, expFiles []string) {
	charmResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(charmResponse.Error, gc.Equals, "")
	c.Check(charmResponse.Files, gc.DeepEquals, expFiles)
}

func (s *charmsCommonSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	charmResponse := s.assertResponse(c, resp, expCode)
	c.Check(charmResponse.Error, gc.Matches, expError)
}

func (s *charmsCommonSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.CharmsResponse {
	body := assertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var charmResponse params.CharmsResponse
	err := json.Unmarshal(body, &charmResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	return charmResponse
}

type charmsSuite struct {
	charmsCommonSuite
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) SetUpSuite(c *gc.C) {
	// TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping this on windows for now")
	}
	s.charmsCommonSuite.SetUpSuite(c)
}

func (s *charmsSuite) TestCharmsServedSecurely(c *gc.C) {
	info := s.APIInfo(c)
	uri := "http://" + info.Addrs[0] + "/charms"
	s.sendRequest(c, httpRequestParams{
		method:      "GET",
		url:         uri,
		expectError: `.*malformed HTTP response.*`,
	})
}

func (s *charmsSuite) TestPOSTRequiresAuth(c *gc.C) {
	resp := s.sendRequest(c, httpRequestParams{method: "POST", url: s.charmsURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "no credentials provided")
}

func (s *charmsSuite) TestGETDoesNotRequireAuth(c *gc.C) {
	resp := s.sendRequest(c, httpRequestParams{method: "GET", url: s.charmsURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected url=CharmURL query argument")
}

func (s *charmsSuite) TestRequiresPOSTorGET(c *gc.C) {
	resp := s.authRequest(c, httpRequestParams{method: "PUT", url: s.charmsURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)
}

func (s *charmsSuite) TestAuthRequiresUser(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	resp := s.sendRequest(c, httpRequestParams{
		tag:      machine.Tag().String(),
		password: password,
		method:   "POST",
		url:      s.charmsURI(c, ""),
		nonce:    "fake_nonce",
	})
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "invalid entity name or password")

	// Now try a user login.
	resp = s.authRequest(c, httpRequestParams{method: "POST", url: s.charmsURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected series=URL argument")
}

func (s *charmsSuite) TestUploadRequiresSeries(c *gc.C) {
	resp := s.authRequest(c, httpRequestParams{method: "POST", url: s.charmsURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected series=URL argument")
}

func (s *charmsSuite) TestUploadFailsWithInvalidZip(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)

	// Pretend we upload a zip by setting the Content-Type, so we can
	// check the error at extraction time later.
	resp := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", tempFile.Name())
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "cannot open charm archive: zip: not a valid zip file")

	// Now try with the default Content-Type.
	resp = s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/octet-stream", tempFile.Name())
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected Content-Type: application/zip, got: application/octet-stream")
}

func (s *charmsSuite) TestUploadBumpsRevision(c *gc.C) {
	// Add the dummy charm with revision 1.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	_, err := s.State.AddCharm(ch, curl, "dummy-storage-path", "dummy-1-sha256")
	c.Assert(err, jc.ErrorIsNil)

	// Now try uploading the same revision and verify it gets bumped,
	// and the BundleSha256 is calculated.
	resp := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", ch.Path)
	expectedURL := charm.MustParseURL("local:quantal/dummy-2")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 2)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	// No more checks for the hash here, because it is
	// verified in TestUploadRespectsLocalRevision.
	c.Assert(sch.BundleSha256(), gc.Not(gc.Equals), "")
}

func (s *charmsSuite) TestUploadRespectsLocalRevision(c *gc.C) {
	// Make a dummy charm dir with revision 123.
	dir := testcharms.Repo.ClonedDir(c.MkDir(), "dummy")
	dir.SetDiskRevision(123)
	// Now bundle the dir.
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = dir.ArchiveTo(tempFile)
	c.Assert(err, jc.ErrorIsNil)

	// Now try uploading it and ensure the revision persists.
	resp := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", tempFile.Name())
	expectedURL := charm.MustParseURL("local:quantal/dummy-123")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 123)
	c.Assert(sch.IsUploaded(), jc.IsTrue)

	// First rewind the reader, which was reset but BundleTo() above.
	_, err = tempFile.Seek(0, 0)
	c.Assert(err, jc.ErrorIsNil)

	// Finally, verify the SHA256.
	expectedSHA256, _, err := utils.ReadSHA256(tempFile)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(sch.BundleSha256(), gc.Equals, expectedSHA256)

	storage := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	reader, _, err := storage.Get(sch.StoragePath())
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()
	downloadedSHA256, _, err := utils.ReadSHA256(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(downloadedSHA256, gc.Equals, expectedSHA256)
}

func (s *charmsSuite) TestUploadAllowsTopLevelPath(c *gc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	// Backwards compatibility check, that we can upload charms to
	// https://host:port/charms
	url := s.charmsURL(c, "series=quantal")
	url.Path = "/charms"
	resp := s.uploadRequest(c, url.String(), "application/zip", ch.Path)
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestUploadAllowsModelUUIDPath(c *gc.C) {
	// Check that we can upload charms to https://host:port/ModelUUID/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL(c, "series=quantal")
	url.Path = fmt.Sprintf("/model/%s/charms", s.modelUUID)
	resp := s.uploadRequest(c, url.String(), "application/zip", ch.Path)
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestUploadAllowsOtherModelUUIDPath(c *gc.C) {
	envState := s.setupOtherModel(c)
	// Check that we can upload charms to https://host:port/ModelUUID/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL(c, "series=quantal")
	url.Path = fmt.Sprintf("/model/%s/charms", envState.ModelUUID())
	resp := s.uploadRequest(c, url.String(), "application/zip", ch.Path)
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestUploadRejectsWrongModelUUIDPath(c *gc.C) {
	// Check that we cannot upload charms to https://host:port/BADModelUUID/charms
	url := s.charmsURL(c, "series=quantal")
	url.Path = "/model/dead-beef-123456/charms"
	resp := s.authRequest(c, httpRequestParams{method: "POST", url: url.String()})
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown model: "dead-beef-123456"`)
}

func (s *charmsSuite) TestUploadRepackagesNestedArchives(c *gc.C) {
	// Make a clone of the dummy charm in a nested directory.
	rootDir := c.MkDir()
	dirPath := filepath.Join(rootDir, "subdir1", "subdir2")
	err := os.MkdirAll(dirPath, 0755)
	c.Assert(err, jc.ErrorIsNil)
	dir := testcharms.Repo.ClonedDir(dirPath, "dummy")
	// Now tweak the path the dir thinks it is in and bundle it.
	dir.Path = rootDir
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = dir.ArchiveTo(tempFile)
	c.Assert(err, jc.ErrorIsNil)

	// Try reading it as a bundle - should fail due to nested dirs.
	_, err = charm.ReadCharmArchive(tempFile.Name())
	c.Assert(err, gc.ErrorMatches, `archive file "metadata.yaml" not found`)

	// Now try uploading it - should succeeed and be repackaged.
	resp := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", tempFile.Name())
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 1)
	c.Assert(sch.IsUploaded(), jc.IsTrue)

	// Get it from the storage and try to read it as a bundle - it
	// should succeed, because it was repackaged during upload to
	// strip nested dirs.
	storage := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	reader, _, err := storage.Get(sch.StoragePath())
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()

	data, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	downloadedFile, err := ioutil.TempFile(c.MkDir(), "downloaded")
	c.Assert(err, jc.ErrorIsNil)
	defer downloadedFile.Close()
	defer os.Remove(downloadedFile.Name())
	err = ioutil.WriteFile(downloadedFile.Name(), data, 0644)
	c.Assert(err, jc.ErrorIsNil)

	bundle, err := charm.ReadCharmArchive(downloadedFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bundle.Revision(), jc.DeepEquals, sch.Revision())
	c.Assert(bundle.Meta(), jc.DeepEquals, sch.Meta())
	c.Assert(bundle.Config(), jc.DeepEquals, sch.Config())
}

func (s *charmsSuite) TestGetRequiresCharmURL(c *gc.C) {
	uri := s.charmsURI(c, "?file=hooks/install")
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		"expected url=CharmURL query argument",
	)
}

func (s *charmsSuite) TestGetFailsWithInvalidCharmURL(c *gc.C) {
	uri := s.charmsURI(c, "?url=local:precise/no-such")
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
	s.assertErrorResponse(
		c, resp, http.StatusNotFound,
		`unable to retrieve and save the charm: cannot get charm from state: charm "local:precise/no-such" not found`,
	)
}

func (s *charmsSuite) TestGetReturnsNotFoundWhenMissing(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", ch.Path)

	// Ensure a 404 is returned for files not included in the charm.
	for i, file := range []string{
		"no-such-file", "..", "../../../etc/passwd", "hooks/delete",
	} {
		c.Logf("test %d: %s", i, file)
		uri := s.charmsURI(c, "?url=local:quantal/dummy-1&file="+file)
		resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
		c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
	}
}

func (s *charmsSuite) TestGetReturnsForbiddenWithDirectory(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", ch.Path)

	// Ensure a 403 is returned if the requested file is a directory.
	uri := s.charmsURI(c, "?url=local:quantal/dummy-1&file=hooks")
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
}

func (s *charmsSuite) TestGetReturnsFileContents(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", ch.Path)

	// Ensure the file contents are properly returned.
	for i, t := range []struct {
		summary  string
		file     string
		response string
	}{{
		summary:  "relative path",
		file:     "revision",
		response: "1",
	}, {
		summary:  "exotic path",
		file:     "./hooks/../revision",
		response: "1",
	}, {
		summary:  "sub-directory path",
		file:     "hooks/install",
		response: "#!/bin/bash\necho \"Done!\"\n",
	},
	} {
		c.Logf("test %d: %s", i, t.summary)
		uri := s.charmsURI(c, "?url=local:quantal/dummy-1&file="+t.file)
		resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
		s.assertGetFileResponse(c, resp, t.response, "text/plain; charset=utf-8")
	}
}

func (s *charmsSuite) TestGetStarReturnsArchiveBytes(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", ch.Path)

	data, err := ioutil.ReadFile(ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	uri := s.charmsURI(c, "?url=local:quantal/dummy-1&file=*")
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
	s.assertGetFileResponse(c, resp, string(data), "application/zip")
}

func (s *charmsSuite) TestGetAllowsTopLevelPath(c *gc.C) {
	// Backwards compatibility check, that we can GET from charms at
	// https://host:port/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", ch.Path)
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = "/charms"
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetAllowsModelUUIDPath(c *gc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", ch.Path)
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = fmt.Sprintf("/model/%s/charms", s.modelUUID)
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetAllowsOtherEnvironment(c *gc.C) {
	envState := s.setupOtherModel(c)

	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", ch.Path)
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = fmt.Sprintf("/model/%s/charms", envState.ModelUUID())
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetRejectsWrongModelUUIDPath(c *gc.C) {
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = "/model/dead-beef-123456/charms"
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: url.String()})
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown model: "dead-beef-123456"`)
}

func (s *charmsSuite) TestGetReturnsManifest(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), "application/zip", ch.Path)

	// Ensure charm files are properly listed.
	uri := s.charmsURI(c, "?url=local:quantal/dummy-1")
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
	manifest, err := ch.Manifest()
	c.Assert(err, jc.ErrorIsNil)
	expectedFiles := manifest.SortedValues()
	s.assertGetFileListResponse(c, resp, expectedFiles)
	ctype := resp.Header.Get("content-type")
	c.Assert(ctype, gc.Equals, params.ContentTypeJSON)
}

func (s *charmsSuite) TestGetUsesCache(c *gc.C) {
	// Add a fake charm archive in the cache directory.
	cacheDir := filepath.Join(s.DataDir(), "charm-get-cache")
	err := os.MkdirAll(cacheDir, 0755)
	c.Assert(err, jc.ErrorIsNil)

	// Create and save a bundle in it.
	charmDir := testcharms.Repo.ClonedDir(c.MkDir(), "dummy")
	testPath := filepath.Join(charmDir.Path, "utils.js")
	contents := "// blah blah"
	err = ioutil.WriteFile(testPath, []byte(contents), 0755)
	c.Assert(err, jc.ErrorIsNil)
	var buffer bytes.Buffer
	err = charmDir.ArchiveTo(&buffer)
	c.Assert(err, jc.ErrorIsNil)
	charmArchivePath := filepath.Join(
		cacheDir, charm.Quote("local:trusty/django-42")+".zip")
	err = ioutil.WriteFile(charmArchivePath, buffer.Bytes(), 0644)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the cached contents are properly retrieved.
	uri := s.charmsURI(c, "?url=local:trusty/django-42&file=utils.js")
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
	s.assertGetFileResponse(c, resp, contents, params.ContentTypeJS)
}

type charmsWithMacaroonsSuite struct {
	charmsCommonSuite
}

var _ = gc.Suite(&charmsWithMacaroonsSuite{})

func (s *charmsWithMacaroonsSuite) SetUpTest(c *gc.C) {
	s.macaroonAuthEnabled = true
	s.authHttpSuite.SetUpTest(c)
}

func (s *charmsWithMacaroonsSuite) TestWithNoBasicAuthReturnsDischargeRequiredError(c *gc.C) {
	resp := s.sendRequest(c, httpRequestParams{
		method: "POST",
		url:    s.charmsURI(c, ""),
	})

	charmResponse := s.assertResponse(c, resp, http.StatusUnauthorized)
	c.Assert(charmResponse.Error, gc.Equals, "verification failed: no macaroons")
	c.Assert(charmResponse.ErrorCode, gc.Equals, params.CodeDischargeRequired)
	c.Assert(charmResponse.ErrorInfo, gc.NotNil)
	c.Assert(charmResponse.ErrorInfo.Macaroon, gc.NotNil)
}

func (s *charmsWithMacaroonsSuite) TestCanPostWithDischargedMacaroon(c *gc.C) {
	checkCount := 0
	s.DischargerLogin = func() string {
		checkCount++
		return s.userTag.Id()
	}
	resp := s.sendRequest(c, httpRequestParams{
		do:     s.doer(),
		method: "POST",
		url:    s.charmsURI(c, ""),
	})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected series=URL argument")
	c.Assert(checkCount, gc.Equals, 1)
}

// doer returns a Do function that can make a bakery request
// appropriate for a charms endpoint.
func (s *charmsWithMacaroonsSuite) doer() func(*http.Request) (*http.Response, error) {
	return bakeryDo(nil, charmsBakeryGetError)
}

// charmsBakeryGetError implements a getError function
// appropriate for passing to httpbakery.Client.DoWithBodyAndCustomError
// for the charms endpoint.
func charmsBakeryGetError(resp *http.Response) error {
	if resp.StatusCode != http.StatusUnauthorized {
		return nil
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Annotatef(err, "cannot read body")
	}
	var charmResp params.CharmsResponse
	if err := json.Unmarshal(data, &charmResp); err != nil {
		return errors.Annotatef(err, "cannot unmarshal body")
	}
	errResp := &params.Error{
		Message: charmResp.Error,
		Code:    charmResp.ErrorCode,
		Info:    charmResp.ErrorInfo,
	}
	if errResp.Code != params.CodeDischargeRequired {
		return errResp
	}
	if errResp.Info == nil {
		return errors.Annotatef(err, "no error info found in discharge-required response error")
	}
	// It's a discharge-required error, so make an appropriate httpbakery
	// error from it.
	return &httpbakery.Error{
		Message: errResp.Message,
		Code:    httpbakery.ErrDischargeRequired,
		Info: &httpbakery.ErrorInfo{
			Macaroon:     errResp.Info.Macaroon,
			MacaroonPath: errResp.Info.MacaroonPath,
		},
	}
}
