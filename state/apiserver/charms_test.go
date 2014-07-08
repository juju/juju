// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/charm"
	charmtesting "github.com/juju/charm/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
)

type charmsSuite struct {
	authHttpSuite
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) SetUpSuite(c *gc.C) {
	s.authHttpSuite.SetUpSuite(c)
	s.archiveContentType = "application/zip"
	s.apiBinding = "charms"
	s.httpClient = utils.GetNonValidatingHTTPClient()
}

func (s *charmsSuite) TestCharmsServedSecurely(c *gc.C) {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	uri := "http://" + info.Addrs[0] + "/charms"
	_, err = s.sendRequest(c, "", "", "GET", uri, "", nil)
	c.Assert(err, gc.ErrorMatches, `.*malformed HTTP response.*`)
}

func (s *charmsSuite) TestCharmsRequiresAuth(c *gc.C) {
	var result params.CharmsResponse
	resp, err := s.sendRequest(c, "", "", "GET", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized", &result)
}

func (s *charmsSuite) TestCharmsRequiresPOSTorGET(c *gc.C) {
	var result params.CharmsResponse
	resp, err := s.authRequest(c, "PUT", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`, &result)
}

func (s *charmsSuite) TestCharmsAuthRequiresUser(c *gc.C) {
	var result params.CharmsResponse
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)

	resp, err := s.sendRequest(c, machine.Tag().String(), password, "GET", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized", &result)

	// Now try a user login.
	resp, err = s.authRequest(c, "GET", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected url=CharmURL query argument", &result)
}

func (s *charmsSuite) TestCharmsUploadRequiresSeries(c *gc.C) {
	var result params.CharmsResponse
	resp, err := s.authRequest(c, "POST", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected series=URL argument", &result)
}

func (s *charmsSuite) TestCharmsUploadFailsWithInvalidZip(c *gc.C) {
	var result params.CharmsResponse
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, gc.IsNil)

	// Pretend we upload a zip by setting the Content-Type, so we can
	// check the error at extraction time later.
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "cannot open charm archive: zip: not a valid zip file", &result)

	// Now try with the default Content-Type.
	resp, err = s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), false, tempFile.Name())
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected Content-Type: application/zip, got: application/octet-stream", &result)
}

func (s *charmsSuite) TestCharmsUploadBumpsRevision(c *gc.C) {
	// Add the dummy charm with revision 1.
	ch := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, gc.IsNil)

	// Now try uploading the same revision and verify it gets bumped,
	// and the BundleURL and BundleSha256 are calculated.
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, gc.IsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-2")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 2)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	// No more checks for these two here, because they
	// are verified in TestCharmsUploadRespectsLocalRevision.
	c.Assert(sch.BundleURL(), gc.Not(gc.Equals), "")
	c.Assert(sch.BundleSha256(), gc.Not(gc.Equals), "")
}

func (s *charmsSuite) TestCharmsUploadRespectsLocalRevision(c *gc.C) {
	// Make a dummy charm dir with revision 123.
	dir := charmtesting.Charms.ClonedDir(c.MkDir(), "dummy")
	dir.SetDiskRevision(123)
	// Now bundle the dir.
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, gc.IsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = dir.BundleTo(tempFile)
	c.Assert(err, gc.IsNil)

	// Now try uploading it and ensure the revision persists.
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, gc.IsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-123")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 123)
	c.Assert(sch.IsUploaded(), jc.IsTrue)

	// First rewind the reader, which was reset but BundleTo() above.
	_, err = tempFile.Seek(0, 0)
	c.Assert(err, gc.IsNil)

	// Finally, verify the SHA256 and uploaded URL.
	expectedSHA256, _, err := utils.ReadSHA256(tempFile)
	c.Assert(err, gc.IsNil)
	name := charm.Quote(expectedURL.String())
	storage, err := environs.GetStorage(s.State)
	c.Assert(err, gc.IsNil)
	expectedUploadURL, err := storage.URL(name)
	c.Assert(err, gc.IsNil)

	c.Assert(sch.BundleURL().String(), gc.Equals, expectedUploadURL)
	c.Assert(sch.BundleSha256(), gc.Equals, expectedSHA256)

	reader, err := storage.Get(name)
	c.Assert(err, gc.IsNil)
	defer reader.Close()
	downloadedSHA256, _, err := utils.ReadSHA256(reader)
	c.Assert(err, gc.IsNil)
	c.Assert(downloadedSHA256, gc.Equals, expectedSHA256)
}

func (s *charmsSuite) TestCharmsUploadAllowsTopLevelPath(c *gc.C) {
	ch := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	// Backwards compatibility check, that we can upload charms to
	// https://host:port/charms
	url := s.charmsURL(c, "series=quantal")
	url.Path = "/charms"
	resp, err := s.uploadRequest(c, url.String(), true, ch.Path)
	c.Assert(err, gc.IsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestCharmsUploadAllowsEnvUUIDPath(c *gc.C) {
	// Check that we can upload charms to https://host:port/ENVUUID/charms
	ch := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	url := s.charmsURL(c, "series=quantal")
	url.Path = fmt.Sprintf("/environment/%s/charms", environ.UUID())
	resp, err := s.uploadRequest(c, url.String(), true, ch.Path)
	c.Assert(err, gc.IsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestCharmsUploadRejectsWrongEnvUUIDPath(c *gc.C) {
	var result params.CharmsResponse
	// Check that we cannot upload charms to https://host:port/BADENVUUID/charms
	url := s.charmsURL(c, "series=quantal")
	url.Path = "/environment/dead-beef-123456/charms"
	resp, err := s.authRequest(c, "POST", url.String(), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`, &result)
}

func (s *charmsSuite) TestCharmsUploadRepackagesNestedArchives(c *gc.C) {
	// Make a clone of the dummy charm in a nested directory.
	rootDir := c.MkDir()
	dirPath := filepath.Join(rootDir, "subdir1", "subdir2")
	err := os.MkdirAll(dirPath, 0755)
	c.Assert(err, gc.IsNil)
	dir := charmtesting.Charms.ClonedDir(dirPath, "dummy")
	// Now tweak the path the dir thinks it is in and bundle it.
	dir.Path = rootDir
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, gc.IsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = dir.BundleTo(tempFile)
	c.Assert(err, gc.IsNil)

	// Try reading it as a bundle - should fail due to nested dirs.
	_, err = charm.ReadBundle(tempFile.Name())
	c.Assert(err, gc.ErrorMatches, "bundle file not found: metadata.yaml")

	// Now try uploading it - should succeeed and be repackaged.
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, gc.IsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 1)
	c.Assert(sch.IsUploaded(), jc.IsTrue)

	// Get it from the storage and try to read it as a bundle - it
	// should succeed, because it was repackaged during upload to
	// strip nested dirs.
	archiveName := strings.TrimPrefix(sch.BundleURL().RequestURI(), "/dummyenv/private/")
	storage, err := environs.GetStorage(s.State)
	c.Assert(err, gc.IsNil)
	reader, err := storage.Get(archiveName)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	data, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	downloadedFile, err := ioutil.TempFile(c.MkDir(), "downloaded")
	c.Assert(err, gc.IsNil)
	defer downloadedFile.Close()
	defer os.Remove(downloadedFile.Name())
	err = ioutil.WriteFile(downloadedFile.Name(), data, 0644)
	c.Assert(err, gc.IsNil)

	bundle, err := charm.ReadBundle(downloadedFile.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(bundle.Revision(), jc.DeepEquals, sch.Revision())
	c.Assert(bundle.Meta(), jc.DeepEquals, sch.Meta())
	c.Assert(bundle.Config(), jc.DeepEquals, sch.Config())
}

func (s *charmsSuite) TestCharmsGetRequiresCharmURL(c *gc.C) {
	var result params.CharmsResponse
	uri := s.charmsURI(c, "?file=hooks/install")
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		"expected url=CharmURL query argument",
		&result,
	)
}

func (s *charmsSuite) TestCharmsGetFailsWithInvalidCharmURL(c *gc.C) {
	var result params.CharmsResponse
	uri := s.charmsURI(c, "?url=local:precise/no-such")
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		"unable to retrieve and save the charm: charm not found in the provider storage: .*",
		&result,
	)
}

func (s *charmsSuite) TestCharmsGetReturnsNotFoundWhenMissing(c *gc.C) {
	// Add the dummy charm.
	ch := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, gc.IsNil)

	// Ensure a 404 is returned for files not included in the charm.
	for i, file := range []string{
		"no-such-file", "..", "../../../etc/passwd", "hooks/delete",
	} {
		c.Logf("test %d: %s", i, file)
		uri := s.charmsURI(c, "?url=local:quantal/dummy-1&file="+file)
		resp, err := s.authRequest(c, "GET", uri, "", nil)
		c.Assert(err, gc.IsNil)
		c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
	}
}

func (s *charmsSuite) TestCharmsGetReturnsForbiddenWithDirectory(c *gc.C) {
	// Add the dummy charm.
	ch := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, gc.IsNil)

	// Ensure a 403 is returned if the requested file is a directory.
	uri := s.charmsURI(c, "?url=local:quantal/dummy-1&file=hooks")
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
}

func (s *charmsSuite) TestCharmsGetReturnsFileContents(c *gc.C) {
	// Add the dummy charm.
	ch := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, gc.IsNil)

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
		c.Assert(err, gc.IsNil)
		s.assertGetFileResponse(c, resp, t.response, "text/plain; charset=utf-8")
	}
}

func (s *charmsSuite) TestCharmsGetAllowsTopLevelPath(c *gc.C) {
	ch := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, gc.IsNil)
	// Backwards compatibility check, that we can GET from charms at
	// https://host:port/charms
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = "/charms"
	resp, err := s.authRequest(c, "GET", url.String(), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestCharmsGetAllowsEnvUUIDPath(c *gc.C) {
	ch := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, gc.IsNil)
	// Check that we can GET from charms at https://host:port/ENVUUID/charms
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = fmt.Sprintf("/environment/%s/charms", environ.UUID())
	resp, err := s.authRequest(c, "GET", url.String(), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestCharmsGetRejectsWrongEnvUUIDPath(c *gc.C) {
	var result params.CharmsResponse
	// Check that we cannot upload charms to https://host:port/BADENVUUID/charms
	url := s.charmsURL(c, "url=local:quantal/dummy-1&file=revision")
	url.Path = "/environment/dead-beef-123456/charms"
	resp, err := s.authRequest(c, "GET", url.String(), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`, &result)
}

func (s *charmsSuite) TestCharmsGetReturnsManifest(c *gc.C) {
	// Add the dummy charm.
	ch := charmtesting.Charms.Bundle(c.MkDir(), "dummy")
	_, err := s.uploadRequest(
		c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, gc.IsNil)

	// Ensure charm files are properly listed.
	uri := s.charmsURI(c, "?url=local:quantal/dummy-1")
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, gc.IsNil)
	manifest, err := ch.Manifest()
	c.Assert(err, gc.IsNil)
	expectedFiles := manifest.SortedValues()
	s.assertGetFileListResponse(c, resp, expectedFiles)
	ctype := resp.Header.Get("content-type")
	c.Assert(ctype, gc.Equals, "application/json")
}

func (s *charmsSuite) TestCharmsGetUsesCache(c *gc.C) {
	// Add a fake charm archive in the cache directory.
	cacheDir := filepath.Join(s.DataDir(), "charm-get-cache")
	err := os.MkdirAll(cacheDir, 0755)
	c.Assert(err, gc.IsNil)

	// Create and save a bundle in it.
	charmDir := charmtesting.Charms.ClonedDir(c.MkDir(), "dummy")
	testPath := filepath.Join(charmDir.Path, "utils.js")
	contents := "// blah blah"
	err = ioutil.WriteFile(testPath, []byte(contents), 0755)
	c.Assert(err, gc.IsNil)
	var buffer bytes.Buffer
	err = charmDir.BundleTo(&buffer)
	c.Assert(err, gc.IsNil)
	charmArchivePath := filepath.Join(
		cacheDir, charm.Quote("local:trusty/django-42")+".zip")
	err = ioutil.WriteFile(charmArchivePath, buffer.Bytes(), 0644)
	c.Assert(err, gc.IsNil)

	// Ensure the cached contents are properly retrieved.
	uri := s.charmsURI(c, "?url=local:trusty/django-42&file=utils.js")
	resp, err := s.authRequest(c, "GET", uri, "", nil)
	c.Assert(err, gc.IsNil)
	s.assertGetFileResponse(c, resp, contents, "application/javascript")
}

func (s *charmsSuite) charmsURL(c *gc.C, query string) *url.URL {
	uri, err := s.baseURL()
	c.Assert(err, gc.IsNil)
	uri.Path += "/charms"
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
	var result params.CharmsResponse
	body := assertResponse(c, resp, http.StatusOK, "application/json")
	jsonResponse(c, body, &result)
	c.Check(result.Error, gc.Equals, "")
	c.Check(result.CharmURL, gc.Equals, expCharmURL)
}

func (s *charmsSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := assertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *charmsSuite) assertGetFileListResponse(c *gc.C, resp *http.Response, expFiles []string) {
	var result params.CharmsResponse
	body := assertResponse(c, resp, http.StatusOK, "application/json")
	jsonResponse(c, body, &result)
	c.Check(result.Error, gc.Equals, "")
	c.Check(result.Files, gc.DeepEquals, expFiles)
}
