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

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testcharms"
)

type charmsSuite struct {
	userAuthHttpSuite
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) SetUpSuite(c *gc.C) {
	// TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping this on windows for now")
	}
	s.userAuthHttpSuite.SetUpSuite(c)
	s.archiveContentType = "application/zip"
}

func (s *charmsSuite) TestCharmsServedSecurely(c *gc.C) {
	info := s.APIInfo(c)
	uri := "http://" + info.Addrs[0] + "/charms"
	_, err := s.sendRequest(c, "", "", "GET", uri, "", nil)
	c.Assert(err, gc.ErrorMatches, `.*malformed HTTP response.*`)
}

func (s *charmsSuite) TestPOSTRequiresAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "POST", s.charmsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")
}

func (s *charmsSuite) TestGETDoesNotRequireAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "GET", s.charmsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected url=CharmURL query argument")
}

func (s *charmsSuite) TestRequiresPOSTorGET(c *gc.C) {
	resp, err := s.authRequest(c, "PUT", s.charmsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
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

	resp, err := s.sendRequest(c, machine.Tag().String(), password, "POST", s.charmsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")

	// Now try a user login.
	resp, err = s.authRequest(c, "POST", s.charmsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected series=URL argument")
}

func (s *charmsSuite) TestUploadRequiresSeries(c *gc.C) {
	resp, err := s.authRequest(c, "POST", s.charmsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected series=URL argument")
}

func (s *charmsSuite) TestUploadFailsWithInvalidZip(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)

	// Pretend we upload a zip by setting the Content-Type, so we can
	// check the error at extraction time later.
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "cannot open charm archive: zip: not a valid zip file")

	// Now try with the default Content-Type.
	resp, err = s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), false, tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
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
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)
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
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
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

	storage := storage.NewStorage(s.State.EnvironUUID(), s.State.MongoSession())
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
	resp, err := s.uploadRequest(c, url.String(), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestUploadAllowsEnvUUIDPath(c *gc.C) {
	// Check that we can upload charms to https://host:port/ENVUUID/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL(c, "series=quantal")
	url.Path = fmt.Sprintf("/environment/%s/charms", s.envUUID)
	resp, err := s.uploadRequest(c, url.String(), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestUploadAllowsOtherEnvUUIDPath(c *gc.C) {
	envState := s.setupOtherEnvironment(c)
	// Check that we can upload charms to https://host:port/ENVUUID/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL(c, "series=quantal")
	url.Path = fmt.Sprintf("/environment/%s/charms", envState.EnvironUUID())
	resp, err := s.uploadRequest(c, url.String(), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestUploadRejectsWrongEnvUUIDPath(c *gc.C) {
	// Check that we cannot upload charms to https://host:port/BADENVUUID/charms
	url := s.charmsURL(c, "series=quantal")
	url.Path = "/environment/dead-beef-123456/charms"
	resp, err := s.authRequest(c, "POST", url.String(), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
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
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
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
	storage := storage.NewStorage(s.State.EnvironUUID(), s.State.MongoSession())
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
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		"expected url=CharmURL query argument",
	)
}

func (s *charmsSuite) TestGetFailsWithInvalidCharmURL(c *gc.C) {
	uri := s.charmsURI(c, "?url=local:precise/no-such")
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(
		c, resp, http.StatusNotFound,
		`unable to retrieve and save the charm: cannot get charm from state: charm "local:precise/no-such" not found`,
	)
}

func (s *charmsSuite) TestGetReturnsNotFoundWhenMissing(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure a 404 is returned for files not included in the charm.
	for i, file := range []string{
		"no-such-file", "..", "../../../etc/passwd", "hooks/delete",
	} {
		c.Logf("test %d: %s", i, file)
		uri := s.charmsURI(c, "?url=local:quantal/dummy-1&file="+file)
		resp, err := s.authRequest(c, "GET", uri, "", nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
	}
}

func (s *charmsSuite) TestGetReturnsForbiddenWithDirectory(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure a 403 is returned if the requested file is a directory.
	uri := s.charmsURI(c, "?url=local:quantal/dummy-1&file=hooks")
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
}

func (s *charmsSuite) TestGetReturnsFileContents(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)

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
		resp, err := s.authRequest(c, "GET", uri, "", nil)
		c.Assert(err, jc.ErrorIsNil)
		s.assertGetFileResponse(c, resp, t.response, "text/plain; charset=utf-8")
	}
}

func (s *charmsSuite) TestGetStarReturnsArchiveBytes(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	data, err := ioutil.ReadFile(ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	uri := s.charmsURI(c, "?url=local:quantal/dummy-1&file=*")
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertGetFileResponse(c, resp, string(data), "application/zip")
}

func (s *charmsSuite) TestGetAllowsTopLevelPath(c *gc.C) {
	// Backwards compatibility check, that we can GET from charms at
	// https://host:port/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = "/charms"
	resp, err := s.authRequest(c, "GET", url.String(), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetAllowsEnvUUIDPath(c *gc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = fmt.Sprintf("/environment/%s/charms", s.envUUID)
	resp, err := s.authRequest(c, "GET", url.String(), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetAllowsOtherEnvironment(c *gc.C) {
	envState := s.setupOtherEnvironment(c)

	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = fmt.Sprintf("/environment/%s/charms", envState.EnvironUUID())
	resp, err := s.authRequest(c, "GET", url.String(), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetRejectsWrongEnvUUIDPath(c *gc.C) {
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = "/environment/dead-beef-123456/charms"
	resp, err := s.authRequest(c, "GET", url.String(), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
}

func (s *charmsSuite) TestGetReturnsManifest(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure charm files are properly listed.
	uri := s.charmsURI(c, "?url=local:quantal/dummy-1")
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	manifest, err := ch.Manifest()
	c.Assert(err, jc.ErrorIsNil)
	expectedFiles := manifest.SortedValues()
	s.assertGetFileListResponse(c, resp, expectedFiles)
	ctype := resp.Header.Get("content-type")
	c.Assert(ctype, gc.Equals, apihttp.CTypeJSON)
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
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertGetFileResponse(c, resp, contents, "application/javascript")
}

func (s *charmsSuite) charmsURL(c *gc.C, query string) *url.URL {
	uri := s.baseURL(c)
	if s.envUUID == "" {
		uri.Path = "/charms"
	} else {
		uri.Path = fmt.Sprintf("/environment/%s/charms", s.envUUID)
	}
	uri.RawQuery = query
	return uri
}

func (s *charmsSuite) charmsURI(c *gc.C, query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.charmsURL(c, query).String()
}

func (s *charmsSuite) assertUploadResponse(c *gc.C, resp *http.Response, expCharmURL string) {
	body := assertResponse(c, resp, http.StatusOK, apihttp.CTypeJSON)
	charmResponse := jsonResponse(c, body)
	c.Check(charmResponse.Error, gc.Equals, "")
	c.Check(charmResponse.CharmURL, gc.Equals, expCharmURL)
}

func (s *charmsSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := assertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *charmsSuite) assertGetFileListResponse(c *gc.C, resp *http.Response, expFiles []string) {
	body := assertResponse(c, resp, http.StatusOK, apihttp.CTypeJSON)
	charmResponse := jsonResponse(c, body)
	c.Check(charmResponse.Error, gc.Equals, "")
	c.Check(charmResponse.Files, gc.DeepEquals, expFiles)
}

func assertResponse(c *gc.C, resp *http.Response, expCode int, expContentType string) []byte {
	c.Check(resp.StatusCode, gc.Equals, expCode)
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	c.Assert(err, jc.ErrorIsNil)
	ctype := resp.Header.Get("Content-Type")
	c.Assert(ctype, gc.Equals, expContentType)
	return body
}

func jsonResponse(c *gc.C, body []byte) (jsonResponse params.CharmsResponse) {
	err := json.Unmarshal(body, &jsonResponse)
	c.Assert(err, jc.ErrorIsNil)
	return
}
