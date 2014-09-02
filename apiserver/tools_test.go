// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/state"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type toolsSuite struct {
	authHttpSuite
}

var _ = gc.Suite(&toolsSuite{})

func (s *toolsSuite) SetUpSuite(c *gc.C) {
	s.authHttpSuite.SetUpSuite(c)
	s.archiveContentType = "application/x-tar-gz"
}

func (s *toolsSuite) TestToolsUploadedSecurely(c *gc.C) {
	info := s.APIInfo(c)
	uri := "http://" + info.Addrs[0] + "/tools"
	_, err := s.sendRequest(c, "", "", "PUT", uri, "", nil)
	c.Assert(err, gc.ErrorMatches, `.*malformed HTTP response.*`)
}

func (s *toolsSuite) TestRequiresAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "GET", s.toolsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")
}

func (s *toolsSuite) TestRequiresPOST(c *gc.C) {
	resp, err := s.authRequest(c, "PUT", s.toolsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)
}

func (s *toolsSuite) TestAuthRequiresUser(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)

	resp, err := s.sendRequest(c, machine.Tag().String(), password, "POST", s.toolsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")

	// Now try a user login.
	resp, err = s.authRequest(c, "POST", s.toolsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestUploadRequiresVersion(c *gc.C) {
	resp, err := s.authRequest(c, "POST", s.toolsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestUploadFailsWithNoTools(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "tools")
	c.Assert(err, gc.IsNil)

	resp, err := s.uploadRequest(c, s.toolsURI(c, "?binaryVersion=1.18.0-quantal-amd64"), true, tempFile.Name())
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "no tools uploaded")
}

func (s *toolsSuite) TestUploadFailsWithInvalidContentType(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "tools")
	c.Assert(err, gc.IsNil)

	// Now try with the default Content-Type.
	resp, err := s.uploadRequest(c, s.toolsURI(c, "?binaryVersion=1.18.0-quantal-amd64"), false, tempFile.Name())
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest, "expected Content-Type: application/x-tar-gz, got: application/octet-stream")
}

func (s *toolsSuite) setupToolsForUpload(c *gc.C) (coretools.List, version.Binary, string) {
	localStorage := c.MkDir()
	vers := version.MustParseBinary("1.9.0-quantal-amd64")
	versionStrings := []string{vers.String()}
	expectedTools := toolstesting.MakeToolsWithCheckSum(c, localStorage, "releases", versionStrings)
	toolsFile := tools.StorageName(vers)
	return expectedTools, vers, path.Join(localStorage, toolsFile)
}

func (s *toolsSuite) TestUpload(c *gc.C) {
	// Make some fake tools.
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	// Now try uploading them.
	resp, err := s.uploadRequest(
		c, s.toolsURI(c, "?binaryVersion="+vers.String()), true, toolPath)
	c.Assert(err, gc.IsNil)

	// Check the response.
	stor := s.Environ.Storage()
	toolsURL, err := stor.URL(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	expectedTools[0].URL = toolsURL
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	r, err := stor.Get(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	uploadedData, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	expectedData, err := ioutil.ReadFile(toolPath)
	c.Assert(err, gc.IsNil)
	c.Assert(uploadedData, gc.DeepEquals, expectedData)
}

func (s *toolsSuite) TestUploadAllowsTopLevelPath(c *gc.C) {
	// Backwards compatibility check, that we can upload tools to
	// https://host:port/tools
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	url := s.toolsURL(c, "binaryVersion="+vers.String())
	url.Path = "/tools"
	resp, err := s.uploadRequest(c, url.String(), true, toolPath)
	c.Assert(err, gc.IsNil)
	// Check the response.
	stor := s.Environ.Storage()
	toolsURL, err := stor.URL(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	expectedTools[0].URL = toolsURL
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadAllowsEnvUUIDPath(c *gc.C) {
	// Check that we can upload tools to https://host:port/ENVUUID/tools
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	url := s.toolsURL(c, "binaryVersion="+vers.String())
	url.Path = fmt.Sprintf("/environment/%s/tools", environ.UUID())
	resp, err := s.uploadRequest(c, url.String(), true, toolPath)
	c.Assert(err, gc.IsNil)
	// Check the response.
	stor := s.Environ.Storage()
	toolsURL, err := stor.URL(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	expectedTools[0].URL = toolsURL
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadRejectsWrongEnvUUIDPath(c *gc.C) {
	// Check that we cannot access the tools at https://host:port/BADENVUUID/tools
	url := s.toolsURL(c, "")
	url.Path = "/environment/dead-beef-123456/tools"
	resp, err := s.authRequest(c, "POST", url.String(), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
}

func (s *toolsSuite) TestUploadSeriesExpanded(c *gc.C) {
	// Make some fake tools.
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	// Now try uploading them. The "series" parameter is accepted
	// but ignored; the API server will expand the tools for all
	// supported series.
	params := "?binaryVersion=" + vers.String() + "&series=nonsense"
	resp, err := s.uploadRequest(c, s.toolsURI(c, params), true, toolPath)
	c.Assert(err, gc.IsNil)

	// Check the response.
	stor := s.Environ.Storage()
	toolsURL, err := stor.URL(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	expectedTools[0].URL = toolsURL
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	for _, series := range version.OSSupportedSeries(version.Ubuntu) {
		toolsVersion := vers
		toolsVersion.Series = series
		r, err := stor.Get(tools.StorageName(toolsVersion))
		c.Assert(err, gc.IsNil)
		uploadedData, err := ioutil.ReadAll(r)
		c.Assert(err, gc.IsNil)
		expectedData, err := ioutil.ReadFile(toolPath)
		c.Assert(err, gc.IsNil)
		c.Assert(uploadedData, gc.DeepEquals, expectedData)
	}
}

func (s *toolsSuite) TestDownloadEnvUUIDPath(c *gc.C) {
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	s.testDownload(c, environ.UUID())
}

func (s *toolsSuite) TestDownloadTopLevelPath(c *gc.C) {
	s.testDownload(c, "")
}

func (s *toolsSuite) testDownload(c *gc.C, uuid string) {
	stor := s.Environ.Storage()
	envtesting.RemoveTools(c, stor)
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, version.Current)[0]

	resp, err := s.downloadRequest(c, tools.Version, uuid)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.HasLen, int(tools.Size))

	hash := sha256.New()
	hash.Write(data)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), gc.Equals, tools.SHA256)
}

func (s *toolsSuite) TestDownloadRejectsWrongEnvUUIDPath(c *gc.C) {
	resp, err := s.downloadRequest(c, version.Current, "dead-beef-123456")
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
}

func (s *toolsSuite) toolsURL(c *gc.C, query string) *url.URL {
	uri := s.baseURL(c)
	uri.Path += "/tools"
	uri.RawQuery = query
	return uri
}

func (s *toolsSuite) toolsURI(c *gc.C, query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.toolsURL(c, query).String()
}

func (s *toolsSuite) downloadRequest(c *gc.C, version version.Binary, uuid string) (*http.Response, error) {
	url := s.toolsURL(c, "")
	if uuid == "" {
		url.Path = fmt.Sprintf("/tools/%s", version)
	} else {
		url.Path = fmt.Sprintf("/environment/%s/tools/%s", uuid, version)
	}
	return s.sendRequest(c, "", "", "GET", url.String(), "", nil)
}

func (s *toolsSuite) assertUploadResponse(c *gc.C, resp *http.Response, agentTools *coretools.Tools) {
	body := assertResponse(c, resp, http.StatusOK, "application/json")
	toolsResult := jsonToolsResponse(c, body)
	c.Check(toolsResult.Error, gc.IsNil)
	c.Check(toolsResult.Tools, gc.DeepEquals, agentTools)
}

func (s *toolsSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := assertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *toolsSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	body := assertResponse(c, resp, expCode, "application/json")
	err := jsonToolsResponse(c, body).Error
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, expError)
}

func jsonToolsResponse(c *gc.C, body []byte) (jsonResponse params.ToolsResult) {
	err := json.Unmarshal(body, &jsonResponse)
	c.Assert(err, gc.IsNil)
	return
}
