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
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	commontesting "github.com/juju/juju/apiserver/common/testing"
	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type toolsSuite struct {
	userAuthHttpSuite
	commontesting.BlockHelper
}

var _ = gc.Suite(&toolsSuite{})

func (s *toolsSuite) SetUpSuite(c *gc.C) {
	s.userAuthHttpSuite.SetUpSuite(c)
	s.archiveContentType = "application/x-tar-gz"
}

func (s *toolsSuite) SetUpTest(c *gc.C) {
	s.userAuthHttpSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

func (s *toolsSuite) TestToolsUploadedSecurely(c *gc.C) {
	info := s.APIInfo(c)
	uri := "http://" + info.Addrs[0] + "/tools"
	_, err := s.sendRequest(c, "", "", "PUT", uri, "", nil)
	c.Assert(err, gc.ErrorMatches, `.*malformed HTTP response.*`)
}

func (s *toolsSuite) TestRequiresAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "GET", s.toolsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")
}

func (s *toolsSuite) TestRequiresPOST(c *gc.C) {
	resp, err := s.authRequest(c, "PUT", s.toolsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)
}

func (s *toolsSuite) TestAuthRequiresUser(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	resp, err := s.sendRequest(c, machine.Tag().String(), password, "POST", s.toolsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")

	// Now try a user login.
	resp, err = s.authRequest(c, "POST", s.toolsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestUploadRequiresVersion(c *gc.C) {
	resp, err := s.authRequest(c, "POST", s.toolsURI(c, ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestUploadFailsWithNoTools(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "tools")
	c.Assert(err, jc.ErrorIsNil)

	resp, err := s.uploadRequest(c, s.toolsURI(c, "?binaryVersion=1.18.0-quantal-amd64"), true, tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "no tools uploaded")
}

func (s *toolsSuite) TestUploadFailsWithInvalidContentType(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "tools")
	c.Assert(err, jc.ErrorIsNil)

	// Now try with the default Content-Type.
	resp, err := s.uploadRequest(c, s.toolsURI(c, "?binaryVersion=1.18.0-quantal-amd64"), false, tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest, "expected Content-Type: application/x-tar-gz, got: application/octet-stream")
}

func (s *toolsSuite) setupToolsForUpload(c *gc.C) (coretools.List, version.Binary, string) {
	localStorage := c.MkDir()
	vers := version.MustParseBinary("1.9.0-quantal-amd64")
	versionStrings := []string{vers.String()}
	expectedTools := toolstesting.MakeToolsWithCheckSum(c, localStorage, "released", versionStrings)
	toolsFile := envtools.StorageName(vers, "released")
	return expectedTools, vers, path.Join(localStorage, toolsFile)
}

func (s *toolsSuite) TestUpload(c *gc.C) {
	// Make some fake tools.
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	// Now try uploading them.
	resp, err := s.uploadRequest(
		c, s.toolsURI(c, "?binaryVersion="+vers.String()), true, toolPath)
	c.Assert(err, jc.ErrorIsNil)

	// Check the response.
	expectedTools[0].URL = fmt.Sprintf("%s/environment/%s/tools/%s", s.baseURL(c), s.State.EnvironUUID(), vers)
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	_, uploadedData := s.getToolsFromStorage(c, s.State, vers)
	expectedData, err := ioutil.ReadFile(toolPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadedData, gc.DeepEquals, expectedData)
}

func (s *toolsSuite) TestBlockUpload(c *gc.C) {
	// Make some fake tools.
	_, vers, toolPath := s.setupToolsForUpload(c)
	// Block all changes.
	s.BlockAllChanges(c, "TestUpload")
	// Now try uploading them.
	resp, err := s.uploadRequest(
		c, s.toolsURI(c, "?binaryVersion="+vers.String()), true, toolPath)
	c.Assert(err, jc.ErrorIsNil)
	problem := s.assertErrorResponse(c, resp, http.StatusBadRequest, "TestUpload")
	s.AssertBlocked(c, problem, "TestUpload")

	// Check the contents.
	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	_, _, err = storage.Tools(vers)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *toolsSuite) TestUploadAllowsTopLevelPath(c *gc.C) {
	// Backwards compatibility check, that we can upload tools to
	// https://host:port/tools
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	url := s.toolsURL(c, "binaryVersion="+vers.String())
	url.Path = "/tools"
	resp, err := s.uploadRequest(c, url.String(), true, toolPath)
	c.Assert(err, jc.ErrorIsNil)
	// Check the response.
	expectedTools[0].URL = fmt.Sprintf("%s/environment/%s/tools/%s", s.baseURL(c), s.State.EnvironUUID(), vers)
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadAllowsEnvUUIDPath(c *gc.C) {
	// Check that we can upload tools to https://host:port/ENVUUID/tools
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	url := s.toolsURL(c, "binaryVersion="+vers.String())
	url.Path = fmt.Sprintf("/environment/%s/tools", s.State.EnvironUUID())
	resp, err := s.uploadRequest(c, url.String(), true, toolPath)
	c.Assert(err, jc.ErrorIsNil)
	// Check the response.
	expectedTools[0].URL = fmt.Sprintf("%s/environment/%s/tools/%s", s.baseURL(c), s.State.EnvironUUID(), vers)
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadAllowsOtherEnvUUIDPath(c *gc.C) {
	envState := s.setupOtherEnvironment(c)
	// Check that we can upload tools to https://host:port/ENVUUID/tools
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	url := s.toolsURL(c, "binaryVersion="+vers.String())
	url.Path = fmt.Sprintf("/environment/%s/tools", envState.EnvironUUID())
	resp, err := s.uploadRequest(c, url.String(), true, toolPath)
	c.Assert(err, jc.ErrorIsNil)
	// Check the response.
	expectedTools[0].URL = fmt.Sprintf("%s/environment/%s/tools/%s", s.baseURL(c), envState.EnvironUUID(), vers)
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadRejectsWrongEnvUUIDPath(c *gc.C) {
	// Check that we cannot access the tools at https://host:port/BADENVUUID/tools
	url := s.toolsURL(c, "")
	url.Path = "/environment/dead-beef-123456/tools"
	resp, err := s.authRequest(c, "POST", url.String(), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
}

func (s *toolsSuite) TestUploadSeriesExpanded(c *gc.C) {
	// Make some fake tools.
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	// Now try uploading them. The tools will be cloned for
	// each additional series specified.
	params := "?binaryVersion=" + vers.String() + "&series=quantal,precise"
	resp, err := s.uploadRequest(c, s.toolsURI(c, params), true, toolPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)

	// Check the response.
	info := s.APIInfo(c)
	expectedTools[0].URL = fmt.Sprintf("%s/environment/%s/tools/%s", s.baseURL(c), info.EnvironTag.Id(), vers)
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	expectedData, err := ioutil.ReadFile(toolPath)
	c.Assert(err, jc.ErrorIsNil)
	for _, series := range []string{"precise", "quantal"} {
		vers := vers
		vers.Series = series
		_, r, err := storage.Tools(vers)
		c.Assert(err, jc.ErrorIsNil)
		uploadedData, err := ioutil.ReadAll(r)
		r.Close()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(uploadedData, gc.DeepEquals, expectedData)
	}

	// ensure other series *aren't* there.
	vers.Series = "trusty"
	_, err = storage.Metadata(vers)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *toolsSuite) TestDownloadEnvUUIDPath(c *gc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", toolstorage.Metadata{
		Version: version.Current,
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, s.State.EnvironUUID())
}

func (s *toolsSuite) TestDownloadOtherEnvUUIDPath(c *gc.C) {
	envState := s.setupOtherEnvironment(c)
	tools := s.storeFakeTools(c, envState, "abc", toolstorage.Metadata{
		Version: version.Current,
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, envState.EnvironUUID())
}

func (s *toolsSuite) TestDownloadTopLevelPath(c *gc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", toolstorage.Metadata{
		Version: version.Current,
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, "")
}

func (s *toolsSuite) TestDownloadFetchesAndCaches(c *gc.C) {
	// The tools are not in toolstorage, so the download request causes
	// the API server to search for the tools in simplestreams, fetch
	// them, and then cache them in toolstorage.
	vers := version.MustParseBinary("1.23.0-trusty-amd64")
	stor := s.DefaultToolsStorage
	envtesting.RemoveTools(c, stor, "released")
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", vers)[0]
	data := s.testDownload(c, tools, "")

	metadata, cachedData := s.getToolsFromStorage(c, s.State, tools.Version)
	c.Assert(metadata.Size, gc.Equals, tools.Size)
	c.Assert(metadata.SHA256, gc.Equals, tools.SHA256)
	c.Assert(string(cachedData), gc.Equals, string(data))
}

func (s *toolsSuite) TestDownloadFetchesAndVerifiesSize(c *gc.C) {
	// Upload fake tools, then upload over the top so the SHA256 hash does not match.
	s.PatchValue(&version.Current.Number, testing.FakeVersionNumber)
	stor := s.DefaultToolsStorage
	envtesting.RemoveTools(c, stor, "released")
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", version.Current)[0]
	err := stor.Put(envtools.StorageName(tools.Version, "released"), strings.NewReader("!"), 1)
	c.Assert(err, jc.ErrorIsNil)

	resp, err := s.downloadRequest(c, tools.Version, "")
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "error fetching tools: size mismatch for .*")
	s.assertToolsNotStored(c, tools.Version)
}

func (s *toolsSuite) TestDownloadFetchesAndVerifiesHash(c *gc.C) {
	// Upload fake tools, then upload over the top so the SHA256 hash does not match.
	s.PatchValue(&version.Current.Number, testing.FakeVersionNumber)
	stor := s.DefaultToolsStorage
	envtesting.RemoveTools(c, stor, "released")
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", version.Current)[0]
	sameSize := strings.Repeat("!", int(tools.Size))
	err := stor.Put(envtools.StorageName(tools.Version, "released"), strings.NewReader(sameSize), tools.Size)
	c.Assert(err, jc.ErrorIsNil)

	resp, err := s.downloadRequest(c, tools.Version, "")
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "error fetching tools: hash mismatch for .*")
	s.assertToolsNotStored(c, tools.Version)
}

func (s *toolsSuite) storeFakeTools(c *gc.C, st *state.State, content string, metadata toolstorage.Metadata) *coretools.Tools {
	storage, err := st.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	err = storage.AddTools(strings.NewReader(content), metadata)
	c.Assert(err, jc.ErrorIsNil)
	return &coretools.Tools{
		Version: metadata.Version,
		Size:    metadata.Size,
		SHA256:  metadata.SHA256,
	}
}

func (s *toolsSuite) getToolsFromStorage(c *gc.C, st *state.State, vers version.Binary) (toolstorage.Metadata, []byte) {
	storage, err := st.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	metadata, r, err := storage.Tools(vers)
	c.Assert(err, jc.ErrorIsNil)
	data, err := ioutil.ReadAll(r)
	r.Close()
	c.Assert(err, jc.ErrorIsNil)
	return metadata, data
}

func (s *toolsSuite) assertToolsNotStored(c *gc.C, vers version.Binary) {
	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	_, err = storage.Metadata(vers)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *toolsSuite) testDownload(c *gc.C, tools *coretools.Tools, uuid string) []byte {
	resp, err := s.downloadRequest(c, tools.Version, uuid)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, int(tools.Size))

	hash := sha256.New()
	hash.Write(data)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), gc.Equals, tools.SHA256)
	return data
}

func (s *toolsSuite) TestDownloadRejectsWrongEnvUUIDPath(c *gc.C) {
	resp, err := s.downloadRequest(c, version.Current, "dead-beef-123456")
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
}

func (s *toolsSuite) toolsURL(c *gc.C, query string) *url.URL {
	uri := s.baseURL(c)
	uri.Path = fmt.Sprintf("/environment/%s/tools", s.envUUID)
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
	body := assertResponse(c, resp, http.StatusOK, apihttp.CTypeJSON)
	toolsResult := jsonToolsResponse(c, body)
	c.Check(toolsResult.Error, gc.IsNil)
	c.Check(toolsResult.Tools, gc.DeepEquals, agentTools)
}

func (s *toolsSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := assertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *toolsSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) error {
	body := assertResponse(c, resp, expCode, apihttp.CTypeJSON)
	err := jsonToolsResponse(c, body).Error
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, expError)
	return err
}

func jsonToolsResponse(c *gc.C, body []byte) (jsonResponse params.ToolsResult) {
	err := json.Unmarshal(body, &jsonResponse)
	c.Assert(err, jc.ErrorIsNil)
	return
}
