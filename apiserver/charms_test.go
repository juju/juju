// Copyright 2012, 2013 Canonical Ltd.
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
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

type charmsSuite struct {
	jujutesting.ApiServerSuite
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) charmsURL(query string) *url.URL {
	url := s.URL(fmt.Sprintf("/model/%s/charms", s.ControllerModelUUID()), nil)
	url.RawQuery = query
	return url
}

func (s *charmsSuite) charmsURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.charmsURL(query).String()
}

func (s *charmsSuite) uploadRequest(c *gc.C, url string, curl string, content io.Reader) *http.Response {
	return sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "PUT",
		URL:         url,
		ContentType: "application/zip",
		Body:        content,
		ExtraHeaders: map[string]string{
			"Juju-Curl": curl,
		},
	})
}

func (s *charmsSuite) objectsCharmsURL(charmRef string) *url.URL {
	return s.URL(fmt.Sprintf("/model-%s/charms/%s", s.ControllerModelUUID(), charmRef), nil)
}

func (s *charmsSuite) objectsCharmsURI(charmRef string) string {
	return s.objectsCharmsURL(charmRef).String()
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

func (s *charmsSuite) TestCharmsServedSecurely(c *gc.C) {
	url := s.charmsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *charmsSuite) TestGETRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.charmsURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *charmsSuite) TestRequiresGET(c *gc.C) {
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.charmsURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "Method Not Allowed\n")
}

func (s *charmsSuite) TestGetRequiresCharmURL(c *gc.C) {
	uri := s.charmsURI("?file=hooks/install")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest,
		".*expected url=CharmURL query argument$",
	)
}

func (s *charmsSuite) TestGetFailsWithInvalidCharmURL(c *gc.C) {
	uri := s.charmsURI("?url=local:precise/no-such")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
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
		resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
		c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
	}
}

func (s *charmsSuite) TestGetReturnsNotYetAvailableForPendingCharms(c *gc.C) {
	// Add a charm in pending mode.
	chInfo := state.CharmInfo{
		ID:          "ch:focal/dummy-1",
		Charm:       testcharms.Repo.CharmArchive(c.MkDir(), "dummy"),
		StoragePath: "", // indicates that we don't have the data in the blobstore yet.
		SHA256:      "", // indicates that we don't have the data in the blobstore yet.
		Version:     "42",
	}
	_, err := s.ControllerModel(c).State().AddCharmMetadata(chInfo)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure a 490 is returned if the charm is pending to be downloaded.
	uri := s.charmsURI("?url=ch:focal/dummy-1")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	c.Assert(resp.StatusCode, gc.Equals, http.StatusConflict, gc.Commentf("expected to get 409 for charm that is pending to be downloaded"))
}

func (s *charmsSuite) TestGetReturnsForbiddenWithDirectory(c *gc.C) {
	// Add the dummy charm.
	chArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	f, err := os.Open(chArchive.Path)
	defer func() { _ = f.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	s.uploadRequest(c, s.objectsCharmsURI("testcharm-"+getCharmHash(c, f)), "local:quantal/testcharm-1", &fileReader{path: chArchive.Path})

	// Ensure a 403 is returned if the requested file is a directory.
	uri := s.charmsURI("?url=local:quantal/testcharm-1&file=hooks")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
}

func (s *charmsSuite) TestGetReturnsFileContents(c *gc.C) {
	// Add the dummy charm.
	chArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	f, err := os.Open(chArchive.Path)
	defer func() { _ = f.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	s.uploadRequest(c, s.objectsCharmsURI("testcharm-"+getCharmHash(c, f)), "local:quantal/testcharm-1", &fileReader{path: chArchive.Path})

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
		uri := s.charmsURI("?url=local:quantal/testcharm-1&file=" + t.file)
		resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
		s.assertGetFileResponse(c, resp, t.response, "text/plain; charset=utf-8")
	}
}

func (s *charmsSuite) TestGetWorksForControllerMachines(c *gc.C) {
	// Make a controller machine.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	const nonce = "noncey"
	m, password := f.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs:  []state.MachineJob{state.JobManageModel},
		Nonce: nonce,
	})

	// Create a hosted model and upload a charm for it.
	newSt := f.MakeModel(c, nil)
	defer newSt.Close()

	curl := "local:quantal/dummy-1"
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := jujutesting.AddCharm(newSt, s.ObjectStore(c, newSt.ModelUUID()), curl, ch, false)
	c.Assert(err, jc.ErrorIsNil)

	// Controller machine should be able to download the charm from
	// the hosted model. This is required for controller workers which
	// are acting on behalf of a particular hosted model.
	url := s.charmsURL("url=" + curl + "&file=revision")
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
	ch, err := charm.ReadCharmDir(
		testcharms.RepoWithSeries("quantal").ClonedDirPath(c.MkDir(), "dummy"))
	c.Assert(err, jc.ErrorIsNil)
	// Create an archive from the charm dir.
	tempFile, err := os.CreateTemp(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = ch.ArchiveTo(tempFile)
	c.Assert(err, jc.ErrorIsNil)
	// Add the dummy charm.
	f, err := os.Open(tempFile.Name())
	defer func() { _ = f.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	s.uploadRequest(c, s.objectsCharmsURI("testcharm-"+getCharmHash(c, f)), "local:quantal/testcharm-1", &fileReader{path: tempFile.Name()})

	data, err := os.ReadFile(tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	uri := s.charmsURI("?url=local:quantal/testcharm-1&file=*")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertGetFileResponse(c, resp, string(data), "application/zip")
}

func (s *charmsSuite) TestGetAllowsModelUUIDPath(c *gc.C) {
	// Add the dummy charm.
	chArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	f, err := os.Open(chArchive.Path)
	defer func() { _ = f.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	s.uploadRequest(c, s.objectsCharmsURI("testcharm-"+getCharmHash(c, f)), "local:quantal/testcharm-1", &fileReader{path: chArchive.Path})

	url := s.charmsURL("url=local:quantal/testcharm-1&file=revision")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetAllowsOtherEnvironment(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	newSt := f.MakeModel(c, nil)
	defer newSt.Close()

	curl := "local:quantal/dummy-1"
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	_, err := jujutesting.AddCharm(newSt, s.ObjectStore(c, newSt.ModelUUID()), curl, ch, false)
	c.Assert(err, jc.ErrorIsNil)

	url := s.charmsURL("url=" + curl + "&file=revision")
	url.Path = fmt.Sprintf("/model/%s/charms", newSt.ModelUUID())
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
	s.assertGetFileResponse(c, resp, "1", "text/plain; charset=utf-8")
}

func (s *charmsSuite) TestGetReturnsManifest(c *gc.C) {
	// Add the dummy charm.
	chArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	f, err := os.Open(chArchive.Path)
	defer func() { _ = f.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	s.uploadRequest(c, s.objectsCharmsURI("testcharm-"+getCharmHash(c, f)), "local:quantal/testcharm-1", &fileReader{path: chArchive.Path})

	// Ensure charm files are properly listed.
	uri := s.charmsURI("?url=local:quantal/testcharm-1")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	manifest, err := chArchive.ArchiveMembers()
	c.Assert(err, jc.ErrorIsNil)
	expectedFiles := manifest.SortedValues()
	s.assertGetFileListResponse(c, resp, expectedFiles)
	ctype := resp.Header.Get("content-type")
	c.Assert(ctype, gc.Equals, params.ContentTypeJSON)
}

type fileReader struct {
	path string
	r    io.Reader
}

func (r *fileReader) Read(out []byte) (int, error) {
	if r.r == nil {
		content, err := os.ReadFile(r.path)
		if err != nil {
			return 0, err
		}
		r.r = bytes.NewReader(content)
	}
	return r.r.Read(out)
}
