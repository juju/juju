// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apitesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
)

type charmsSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) charmsURL(query string) *url.URL {
	url := s.URL(fmt.Sprintf("/model/%s/charms", s.State.ModelUUID()), nil)
	url.RawQuery = query
	return url
}

func (s *charmsSuite) charmsURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.charmsURL(query).String()
}

func (s *charmsSuite) uploadRequest(c *gc.C, url, contentType string, content io.Reader) *http.Response {
	return s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url,
		ContentType: contentType,
		Body:        content,
	})
}

func (s *charmsSuite) assertUploadResponse(c *gc.C, resp *http.Response, expCharmURL string) {
	charmResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(charmResponse.Error, gc.Equals, "")
	c.Check(charmResponse.CharmURL, gc.Equals, expCharmURL)
}

func (s *charmsSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := apitesting.AssertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *charmsSuite) assertGetFileListResponse(c *gc.C, resp *http.Response, expFiles []string) {
	charmResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(charmResponse.Error, gc.Equals, "")
	c.Check(charmResponse.Files, gc.DeepEquals, expFiles)
}

func (s *charmsSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	charmResponse := s.assertResponse(c, resp, expCode)
	c.Check(charmResponse.Error, gc.Matches, expError)
}

func (s *charmsSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.CharmsResponse {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var charmResponse params.CharmsResponse
	err := json.Unmarshal(body, &charmResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	return charmResponse
}

func (s *charmsSuite) setModelImporting(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmsSuite) SetUpSuite(c *gc.C) {
	// TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping this on windows for now")
	}
	s.apiserverBaseSuite.SetUpSuite(c)
}

func (s *charmsSuite) TestCharmsServedSecurely(c *gc.C) {
	url := s.charmsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "GET",
		URL:         url.String(),
		ExpectError: `.*malformed HTTP response.*`,
	})
}

func (s *charmsSuite) TestPOSTRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.charmsURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *charmsSuite) TestGETRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.charmsURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *charmsSuite) TestRequiresPOSTorGET(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.charmsURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "Method Not Allowed\n")
}

func (s *charmsSuite) TestPOSTRequiresUserAuth(c *gc.C) {
	// Add a machine and try to login.
	machine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "noncy",
	})
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:         machine.Tag().String(),
		Password:    password,
		Method:      "POST",
		URL:         s.charmsURI(""),
		Nonce:       "noncy",
		ContentType: "foo/bar",
	})
	body := apitesting.AssertResponse(c, resp, http.StatusForbidden, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authorization failed: tag kind machine not valid\n")

	// Now try a user login.
	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.charmsURI("")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*expected Content-Type: application/zip.+")
}

func (s *charmsSuite) TestUploadFailsWithInvalidZip(c *gc.C) {
	var empty bytes.Buffer

	// Pretend we upload a zip by setting the Content-Type, so we can
	// check the error at extraction time later.
	resp := s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &empty)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*cannot open charm archive: zip: not a valid zip file$")

	// Now try with the default Content-Type.
	resp = s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/octet-stream", &empty)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, ".*expected Content-Type: application/zip, got: application/octet-stream$")
}

func (s *charmsSuite) TestUploadBumpsRevision(c *gc.C) {
	// Add the dummy charm with revision 1.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-storage-path",
		SHA256:      "dummy-1-sha256",
	}
	_, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// Now try uploading the same revision and verify it gets bumped,
	// and the BundleSha256 is calculated.
	f, err := os.Open(ch.Path)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	resp := s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", f)
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
	var buf bytes.Buffer
	err := dir.ArchiveTo(&buf)
	c.Assert(err, jc.ErrorIsNil)
	hash := sha256.New()
	hash.Write(buf.Bytes())
	expectedSHA256 := hex.EncodeToString(hash.Sum(nil))

	// Now try uploading it and ensure the revision persists.
	resp := s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &buf)
	expectedURL := charm.MustParseURL("local:quantal/dummy-123")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 123)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	c.Assert(sch.BundleSha256(), gc.Equals, expectedSHA256)

	storage := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	reader, _, err := storage.Get(sch.StoragePath())
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()
	downloadedSHA256, _, err := utils.ReadSHA256(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(downloadedSHA256, gc.Equals, expectedSHA256)
}

func (s *charmsSuite) TestUploadWithMultiSeriesCharm(c *gc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	resp := s.uploadRequest(c, s.charmsURL("").String(), "application/zip", &fileReader{path: ch.Path})
	expectedURL := charm.MustParseURL("local:dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestUploadAllowsTopLevelPath(c *gc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	// Backwards compatibility check, that we can upload charms to
	// https://host:port/charms
	url := s.charmsURL("series=quantal")
	url.Path = "/charms"
	resp := s.uploadRequest(c, url.String(), "application/zip", &fileReader{path: ch.Path})
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestUploadAllowsModelUUIDPath(c *gc.C) {
	// Check that we can upload charms to https://host:port/ModelUUID/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL("series=quantal")
	resp := s.uploadRequest(c, url.String(), "application/zip", &fileReader{path: ch.Path})
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
}

func (s *charmsSuite) TestUploadAllowsOtherModelUUIDPath(c *gc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	// Check that we can upload charms to https://host:port/ModelUUID/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL("series=quantal")
	url.Path = fmt.Sprintf("/model/%s/charms", newSt.ModelUUID())
	resp := s.uploadRequest(c, url.String(), "application/zip", &fileReader{path: ch.Path})
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
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
	var buf bytes.Buffer
	err = dir.ArchiveTo(&buf)
	c.Assert(err, jc.ErrorIsNil)

	// Try reading it as a bundle - should fail due to nested dirs.
	_, err = charm.ReadCharmArchiveBytes(buf.Bytes())
	c.Assert(err, gc.ErrorMatches, `archive file "metadata.yaml" not found`)

	// Now try uploading it - should succeed and be repackaged.
	resp := s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &buf)
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

func (s *charmsSuite) TestNonLocalCharmUploadFailsIfNotMigrating(c *gc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("cs:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-storage-path",
		SHA256:      "dummy-1-sha256",
	}
	_, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	resp := s.uploadRequest(c, s.charmsURI("?schema=cs&series=quantal"), "application/zip", &fileReader{path: ch.Path})
	s.assertErrorResponse(c, resp, 400, ".*cs charms may only be uploaded during model migration import$")
}

func (s *charmsSuite) TestNonLocalCharmUpload(c *gc.C) {
	// Check that upload of charms with the "cs:" schema works (for
	// model migrations).
	s.setModelImporting(c)
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")

	resp := s.uploadRequest(c, s.charmsURI("?schema=cs&series=quantal"), "application/zip", &fileReader{path: ch.Path})

	expectedURL := charm.MustParseURL("cs:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 1)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
}

func (s *charmsSuite) TestUnsupportedSchema(c *gc.C) {
	s.setModelImporting(c)
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")

	resp := s.uploadRequest(c, s.charmsURI("?schema=zz"), "application/zip", &fileReader{path: ch.Path})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`cannot upload charm: unsupported schema "zz"`,
	)
}

func (s *charmsSuite) TestCharmUploadWithUserOverride(c *gc.C) {
	s.setModelImporting(c)
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")

	resp := s.uploadRequest(c, s.charmsURI("?schema=cs&user=bobo"), "application/zip", &fileReader{path: ch.Path})

	expectedURL := charm.MustParseURL("cs:~bobo/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
}

func (s *charmsSuite) TestNonLocalCharmUploadWithRevisionOverride(c *gc.C) {
	s.setModelImporting(c)
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")

	resp := s.uploadRequest(c, s.charmsURI("?schema=cs&&revision=99"), "application/zip", &fileReader{path: ch.Path})

	expectedURL := charm.MustParseURL("cs:dummy-99")
	s.assertUploadResponse(c, resp, expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 99)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
}

func (s *charmsSuite) TestMigrateCharm(c *gc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()
	importedModel, err := newSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = importedModel.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	// The default user is just a normal user, not a controller admin
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL("series=quantal")
	url.Path = "/migrate/charms"
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url.String(),
		ContentType: "application/zip",
		Body:        &fileReader{path: ch.Path},
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: importedModel.UUID(),
		},
	})
	expectedURL := charm.MustParseURL("local:quantal/dummy-1")
	s.assertUploadResponse(c, resp, expectedURL.String())

	// The charm was added to the migrated model.
	_, err = newSt.Charm(expectedURL)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmsSuite) TestMigrateCharmNotMigrating(c *gc.C) {
	migratedModel := s.Factory.MakeModel(c, nil)
	defer migratedModel.Close()

	// The default user is just a normal user, not a controller admin
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	url := s.charmsURL("series=quantal")
	url.Path = "/migrate/charms"
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url.String(),
		ContentType: "application/zip",
		Body:        &fileReader{path: ch.Path},
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: migratedModel.ModelUUID(),
		},
	})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		`cannot upload charm: model migration mode is "" instead of "importing"`,
	)
}

func (s *charmsSuite) TestMigrateCharmUnauthorized(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "hunter2"})
	url := s.charmsURL("series=quantal")
	url.Path = "/migrate/charms"
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "POST",
		URL:      url.String(),
		Tag:      user.Tag().String(),
		Password: "hunter2",
	})
	body := apitesting.AssertResponse(c, resp, http.StatusForbidden, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Matches, "authorization failed: user .* not a controller admin\n")
}

func (s *charmsSuite) TestGetRequiresCharmURL(c *gc.C) {
	uri := s.charmsURI("?file=hooks/install")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		".*expected url=CharmURL query argument$",
	)
}

func (s *charmsSuite) TestGetFailsWithInvalidCharmURL(c *gc.C) {
	uri := s.charmsURI("?url=local:precise/no-such")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusNotFound,
		`.*cannot get charm from state: charm "local:precise/no-such" not found$`,
	)
}

func (s *charmsSuite) TestGetReturnsNotFoundWhenMissing(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Ensure a 404 is returned for files not included in the charm.
	for i, file := range []string{
		"no-such-file", "..", "../../../etc/passwd", "hooks/delete",
	} {
		c.Logf("test %d: %s", i, file)
		uri := s.charmsURI("?url=local:quantal/dummy-1&file=" + file)
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
		c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
	}
}

func (s *charmsSuite) TestGetReturnsForbiddenWithDirectory(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Ensure a 403 is returned if the requested file is a directory.
	uri := s.charmsURI("?url=local:quantal/dummy-1&file=hooks")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
}

func (s *charmsSuite) TestGetReturnsFileContents(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

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
		uri := s.charmsURI("?url=local:quantal/dummy-1&file=" + t.file)
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
		s.assertGetFileResponse(c, resp, t.response, "text/plain; charset=utf-8")
	}
}

func (s *charmsSuite) TestGetCharmIcon(c *gc.C) {
	// Upload the local charms.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "mysql")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})
	ch = testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Prepare the tests.
	svgMimeType := mime.TypeByExtension(".svg")
	iconPath := filepath.Join(testcharms.Repo.CharmDirPath("mysql"), "icon.svg")
	icon, err := ioutil.ReadFile(iconPath)
	c.Assert(err, jc.ErrorIsNil)
	tests := []struct {
		about      string
		query      string
		expectType string
		expectBody string
	}{{
		about:      "icon found",
		query:      "?url=local:quantal/mysql-1&file=icon.svg",
		expectBody: string(icon),
	}, {
		about: "icon not found",
		query: "?url=local:quantal/dummy-1&file=icon.svg",
	}, {
		about:      "default icon requested: icon found",
		query:      "?url=local:quantal/mysql-1&icon=1",
		expectBody: string(icon),
	}, {
		about:      "default icon requested: icon not found",
		query:      "?url=local:quantal/dummy-1&icon=1",
		expectBody: common.DefaultCharmIcon,
	}, {
		about:      "default icon request ignored",
		query:      "?url=local:quantal/mysql-1&file=revision&icon=1",
		expectType: "text/plain; charset=utf-8",
		expectBody: "1",
	}}

	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		uri := s.charmsURI(test.query)
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
		if test.expectBody == "" {
			s.assertErrorResponse(c, resp, http.StatusNotFound, ".*charm file not found$")
			continue
		}
		if test.expectType == "" {
			test.expectType = svgMimeType
		}
		s.assertGetFileResponse(c, resp, test.expectBody, test.expectType)
	}
}

func (s *charmsSuite) TestGetWorksForControllerMachines(c *gc.C) {
	// Make a controller machine.
	const nonce = "noncey"
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs:  []state.MachineJob{state.JobManageModel},
		Nonce: nonce,
	})

	// Create a hosted model and upload a charm for it.
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	curl := charm.MustParseURL("local:quantal/dummy-1")
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := jujutesting.AddCharm(newSt, curl, ch)
	c.Assert(err, jc.ErrorIsNil)

	// Controller machine should be able to download the charm from
	// the hosted model. This is required for controller workers which
	// are acting on behalf of a particular hosted model.
	url := s.charmsURL("url=" + curl.String() + "&file=revision")
	url.Path = fmt.Sprintf("/model/%s/charms", newSt.ModelUUID())
	params := apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      url.String(),
		Tag:      m.Tag().String(),
		Password: password,
		Nonce:    nonce,
	}
	resp := apitesting.SendHTTPRequest(c, params)
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetStarReturnsArchiveBytes(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	data, err := ioutil.ReadFile(ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	uri := s.charmsURI("?url=local:quantal/dummy-1&file=*")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertGetFileResponse(c, resp, string(data), "application/zip")
}

func (s *charmsSuite) TestGetAllowsTopLevelPath(c *gc.C) {
	// Backwards compatibility check, that we can GET from charms at
	// https://host:port/charms
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})
	url := s.charmsURL("url=local:quantal/dummy-1&file=revision")
	url.Path = "/charms"
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetAllowsModelUUIDPath(c *gc.C) {
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})
	url := s.charmsURL("url=local:quantal/dummy-1&file=revision")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetAllowsOtherEnvironment(c *gc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	curl := charm.MustParseURL("local:quantal/dummy-1")
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := jujutesting.AddCharm(newSt, curl, ch)
	c.Assert(err, jc.ErrorIsNil)

	url := s.charmsURL("url=" + curl.String() + "&file=revision")
	url.Path = fmt.Sprintf("/model/%s/charms", newSt.ModelUUID())
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetReturnsManifest(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Ensure charm files are properly listed.
	uri := s.charmsURI("?url=local:quantal/dummy-1")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	manifest, err := ch.Manifest()
	c.Assert(err, jc.ErrorIsNil)
	expectedFiles := manifest.SortedValues()
	s.assertGetFileListResponse(c, resp, expectedFiles)
	ctype := resp.Header.Get("content-type")
	c.Assert(ctype, gc.Equals, params.ContentTypeJSON)
}

func (s *charmsSuite) TestNoTempFilesLeftBehind(c *gc.C) {
	// Add the dummy charm.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	s.uploadRequest(c, s.charmsURI("?series=quantal"), "application/zip", &fileReader{path: ch.Path})

	// Download it.
	uri := s.charmsURI("?url=local:quantal/dummy-1&file=*")
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	apitesting.AssertResponse(c, resp, http.StatusOK, "application/zip")

	// Ensure the tmp directory exists but nothing is in it.
	files, err := ioutil.ReadDir(filepath.Join(s.config.DataDir, "charm-get-tmp"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(files, gc.HasLen, 0)
}

type fileReader struct {
	path string
	r    io.Reader
}

func (r *fileReader) Read(out []byte) (int, error) {
	if r.r == nil {
		content, err := ioutil.ReadFile(r.path)
		if err != nil {
			return 0, err
		}
		r.r = bytes.NewReader(content)
	}
	return r.r.Read(out)
}
