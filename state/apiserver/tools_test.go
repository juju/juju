// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/tools"
	toolstesting "launchpad.net/juju-core/environs/tools/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
	"path/filepath"
)

type toolsSuite struct {
	authHttpSuite
}

var _ = gc.Suite(&toolsSuite{})

func (s *toolsSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.archiveContentType = "application/x-tar-gz"
}

func (s *toolsSuite) TestToolsUploadedSecurely(c *gc.C) {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	uri := "http://" + info.Addrs[0] + "/tools"
	_, err = s.sendRequest(c, "", "", "PUT", uri, "", nil)
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

	resp, err := s.sendRequest(c, machine.Tag(), password, "POST", s.toolsURI(c, ""), "", nil)
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

func (s *toolsSuite) TestUpload(c *gc.C) {
	// Make some fake tools.
	localStorage := c.MkDir()
	vers := version.MustParseBinary("1.9.0-quantal-amd64")
	versionStrings := []string{vers.String()}
	expectedTools := toolstesting.MakeToolsWithCheckSum(c, localStorage, "releases", versionStrings)

	// Now try uploading them.
	toolsFile := tools.StorageName(vers)
	resp, err := s.uploadRequest(
		c, s.toolsURI(c, "?binaryVersion="+vers.String()), true, path.Join(localStorage, toolsFile))
	c.Assert(err, gc.IsNil)

	// Check the response.
	stor := s.Conn.Environ.Storage()
	toolsURL, err := stor.URL(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	expectedTools[0].URL = toolsURL
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	r, err := stor.Get(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	uploadedData, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	expectedData, err := ioutil.ReadFile(filepath.Join(localStorage, tools.StorageName(vers)))
	c.Assert(err, gc.IsNil)
	c.Assert(uploadedData, gc.DeepEquals, expectedData)
}

func (s *toolsSuite) TestUploadFakeSeries(c *gc.C) {
	// Make some fake tools.
	localStorage := c.MkDir()
	vers := version.MustParseBinary("1.9.0-quantal-amd64")
	versionStrings := []string{vers.String()}
	expectedTools := toolstesting.MakeToolsWithCheckSum(c, localStorage, "releases", versionStrings)

	// Now try uploading them.
	toolsFile := tools.StorageName(vers)
	params := "?binaryVersion=" + vers.String() + "&series=precise,trusty"
	resp, err := s.uploadRequest(c, s.toolsURI(c, params), true, path.Join(localStorage, toolsFile))
	c.Assert(err, gc.IsNil)

	// Check the response.
	stor := s.Conn.Environ.Storage()
	toolsURL, err := stor.URL(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	expectedTools[0].URL = toolsURL
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	for _, series := range []string{"precise", "quantal", "trusty"} {
		toolsVersion := vers
		toolsVersion.Series = series
		r, err := stor.Get(tools.StorageName(toolsVersion))
		c.Assert(err, gc.IsNil)
		uploadedData, err := ioutil.ReadAll(r)
		c.Assert(err, gc.IsNil)
		expectedData, err := ioutil.ReadFile(filepath.Join(localStorage, tools.StorageName(vers)))
		c.Assert(err, gc.IsNil)
		c.Assert(uploadedData, gc.DeepEquals, expectedData)
	}
}

func (s *toolsSuite) toolsURI(c *gc.C, query string) string {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	return "https://" + info.Addrs[0] + "/tools" + query
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
