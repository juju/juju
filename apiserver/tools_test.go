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
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	commontesting "github.com/juju/juju/apiserver/common/testing"
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

// charmsCommonSuite wraps authHttpSuite and adds
// some helper methods suitable for working with the
// tools endpoint.
type toolsCommonSuite struct {
	authHttpSuite
}

func (s *toolsCommonSuite) toolsURL(c *gc.C, query string) *url.URL {
	uri := s.baseURL(c)
	uri.Path = fmt.Sprintf("/model/%s/tools", s.modelUUID)
	uri.RawQuery = query
	return uri
}

func (s *toolsCommonSuite) toolsURI(c *gc.C, query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.toolsURL(c, query).String()
}

func (s *toolsCommonSuite) downloadRequest(c *gc.C, version version.Binary, uuid string) *http.Response {
	url := s.toolsURL(c, "")
	if uuid == "" {
		url.Path = fmt.Sprintf("/tools/%s", version)
	} else {
		url.Path = fmt.Sprintf("/model/%s/tools/%s", uuid, version)
	}
	return s.sendRequest(c, httpRequestParams{method: "GET", url: url.String()})
}

func (s *toolsCommonSuite) assertUploadResponse(c *gc.C, resp *http.Response, agentTools *coretools.Tools) {
	toolsResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(toolsResponse.Error, gc.IsNil)
	c.Check(toolsResponse.Tools, gc.DeepEquals, agentTools)
}

func (s *toolsCommonSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := assertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *toolsCommonSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	toolsResponse := s.assertResponse(c, resp, expCode)
	c.Assert(toolsResponse.Error, gc.NotNil)
	c.Assert(toolsResponse.Error.Message, gc.Matches, expError)
}

func (s *toolsCommonSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.ToolsResult {
	body := assertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var toolsResponse params.ToolsResult
	err := json.Unmarshal(body, &toolsResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	return toolsResponse
}

type toolsSuite struct {
	toolsCommonSuite
	commontesting.BlockHelper
}

var _ = gc.Suite(&toolsSuite{})

func (s *toolsSuite) SetUpTest(c *gc.C) {
	s.toolsCommonSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

func (s *toolsSuite) TestToolsUploadedSecurely(c *gc.C) {
	info := s.APIInfo(c)
	uri := "http://" + info.Addrs[0] + "/tools"
	s.sendRequest(c, httpRequestParams{
		method:      "PUT",
		url:         uri,
		expectError: `.*malformed HTTP response.*`,
	})
}

func (s *toolsSuite) TestRequiresAuth(c *gc.C) {
	resp := s.sendRequest(c, httpRequestParams{method: "GET", url: s.toolsURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "no credentials provided")
}

func (s *toolsSuite) TestRequiresPOST(c *gc.C) {
	resp := s.authRequest(c, httpRequestParams{method: "PUT", url: s.toolsURI(c, "")})
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

	resp := s.sendRequest(c, httpRequestParams{tag: machine.Tag().String(), password: password, method: "POST", url: s.toolsURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "machine 0 not provisioned")

	// Now try a user login.
	resp = s.authRequest(c, httpRequestParams{method: "POST", url: s.toolsURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestUploadRequiresVersion(c *gc.C) {
	resp := s.authRequest(c, httpRequestParams{method: "POST", url: s.toolsURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestUploadFailsWithNoTools(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "tools")
	c.Assert(err, jc.ErrorIsNil)

	resp := s.uploadRequest(c, s.toolsURI(c, "?binaryVersion=1.18.0-quantal-amd64"), "application/x-tar-gz", tempFile.Name())
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "no tools uploaded")
}

func (s *toolsSuite) TestUploadFailsWithInvalidContentType(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "tools")
	c.Assert(err, jc.ErrorIsNil)

	// Now try with the default Content-Type.
	resp := s.uploadRequest(c, s.toolsURI(c, "?binaryVersion=1.18.0-quantal-amd64"), "application/octet-stream", tempFile.Name())
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
	resp := s.uploadRequest(
		c, s.toolsURI(c, "?binaryVersion="+vers.String()), "application/x-tar-gz", toolPath)

	// Check the response.
	expectedTools[0].URL = fmt.Sprintf("%s/model/%s/tools/%s", s.baseURL(c), s.State.ModelUUID(), vers)
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	metadata, uploadedData := s.getToolsFromStorage(c, s.State, vers)
	expectedData, err := ioutil.ReadFile(toolPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadedData, gc.DeepEquals, expectedData)
	allMetadata := s.getToolsMetadataFromStorage(c, s.State)
	c.Assert(allMetadata, jc.DeepEquals, []toolstorage.Metadata{metadata})
}

func (s *toolsSuite) TestBlockUpload(c *gc.C) {
	// Make some fake tools.
	_, vers, toolPath := s.setupToolsForUpload(c)
	// Block all changes.
	s.BlockAllChanges(c, "TestUpload")
	// Now try uploading them.
	resp := s.uploadRequest(
		c, s.toolsURI(c, "?binaryVersion="+vers.String()), "application/x-tar-gz", toolPath)
	toolsResponse := s.assertResponse(c, resp, http.StatusBadRequest)
	s.AssertBlocked(c, toolsResponse.Error, "TestUpload")

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
	resp := s.uploadRequest(c, url.String(), "application/x-tar-gz", toolPath)
	// Check the response.
	expectedTools[0].URL = fmt.Sprintf("%s/model/%s/tools/%s", s.baseURL(c), s.State.ModelUUID(), vers)
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadAllowsModelUUIDPath(c *gc.C) {
	// Check that we can upload tools to https://host:port/ModelUUID/tools
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	url := s.toolsURL(c, "binaryVersion="+vers.String())
	url.Path = fmt.Sprintf("/model/%s/tools", s.State.ModelUUID())
	resp := s.uploadRequest(c, url.String(), "application/x-tar-gz", toolPath)
	// Check the response.
	expectedTools[0].URL = fmt.Sprintf("%s/model/%s/tools/%s", s.baseURL(c), s.State.ModelUUID(), vers)
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadAllowsOtherModelUUIDPath(c *gc.C) {
	envState := s.setupOtherModel(c)
	// Check that we can upload tools to https://host:port/ModelUUID/tools
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	url := s.toolsURL(c, "binaryVersion="+vers.String())
	url.Path = fmt.Sprintf("/model/%s/tools", envState.ModelUUID())
	resp := s.uploadRequest(c, url.String(), "application/x-tar-gz", toolPath)
	// Check the response.
	expectedTools[0].URL = fmt.Sprintf("%s/model/%s/tools/%s", s.baseURL(c), envState.ModelUUID(), vers)
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadRejectsWrongModelUUIDPath(c *gc.C) {
	// Check that we cannot access the tools at https://host:port/BADModelUUID/tools
	url := s.toolsURL(c, "")
	url.Path = "/model/dead-beef-123456/tools"
	resp := s.authRequest(c, httpRequestParams{method: "POST", url: url.String()})
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown model: "dead-beef-123456"`)
}

func (s *toolsSuite) TestUploadSeriesExpanded(c *gc.C) {
	// Make some fake tools.
	expectedTools, vers, toolPath := s.setupToolsForUpload(c)
	// Now try uploading them. The tools will be cloned for
	// each additional series specified.
	params := "?binaryVersion=" + vers.String() + "&series=quantal,precise"
	resp := s.uploadRequest(c, s.toolsURI(c, params), "application/x-tar-gz", toolPath)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)

	// Check the response.
	info := s.APIInfo(c)
	expectedTools[0].URL = fmt.Sprintf("%s/model/%s/tools/%s", s.baseURL(c), info.ModelTag.Id(), vers)
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

func (s *toolsSuite) TestDownloadModelUUIDPath(c *gc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", toolstorage.Metadata{
		Version: version.Binary{
			Number: version.Current,
			Arch:   arch.HostArch(),
			Series: series.HostSeries(),
		},
		Size:   3,
		SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, s.State.ModelUUID())
}

func (s *toolsSuite) TestDownloadOtherModelUUIDPath(c *gc.C) {
	envState := s.setupOtherModel(c)
	tools := s.storeFakeTools(c, envState, "abc", toolstorage.Metadata{
		Version: version.Binary{
			Number: version.Current,
			Arch:   arch.HostArch(),
			Series: series.HostSeries(),
		},
		Size:   3,
		SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, envState.ModelUUID())
}

func (s *toolsSuite) TestDownloadTopLevelPath(c *gc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", toolstorage.Metadata{
		Version: version.Binary{
			Number: version.Current,
			Arch:   arch.HostArch(),
			Series: series.HostSeries(),
		},
		Size:   3,
		SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
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
	s.PatchValue(&version.Current, testing.FakeVersionNumber)
	stor := s.DefaultToolsStorage
	envtesting.RemoveTools(c, stor, "released")
	current := version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", current)[0]
	err := stor.Put(envtools.StorageName(tools.Version, "released"), strings.NewReader("!"), 1)
	c.Assert(err, jc.ErrorIsNil)

	resp := s.downloadRequest(c, tools.Version, "")
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "error fetching tools: size mismatch for .*")
	s.assertToolsNotStored(c, tools.Version)
}

func (s *toolsSuite) TestDownloadFetchesAndVerifiesHash(c *gc.C) {
	// Upload fake tools, then upload over the top so the SHA256 hash does not match.
	s.PatchValue(&version.Current, testing.FakeVersionNumber)
	stor := s.DefaultToolsStorage
	envtesting.RemoveTools(c, stor, "released")
	current := version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", current)[0]
	sameSize := strings.Repeat("!", int(tools.Size))
	err := stor.Put(envtools.StorageName(tools.Version, "released"), strings.NewReader(sameSize), tools.Size)
	c.Assert(err, jc.ErrorIsNil)

	resp := s.downloadRequest(c, tools.Version, "")
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

func (s *toolsSuite) getToolsMetadataFromStorage(c *gc.C, st *state.State) []toolstorage.Metadata {
	storage, err := st.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	metadata, err := storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	return metadata
}

func (s *toolsSuite) assertToolsNotStored(c *gc.C, vers version.Binary) {
	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	_, err = storage.Metadata(vers)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *toolsSuite) testDownload(c *gc.C, tools *coretools.Tools, uuid string) []byte {
	resp := s.downloadRequest(c, tools.Version, uuid)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, int(tools.Size))

	hash := sha256.New()
	hash.Write(data)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), gc.Equals, tools.SHA256)
	return data
}

func (s *toolsSuite) TestDownloadRejectsWrongModelUUIDPath(c *gc.C) {
	current := version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	resp := s.downloadRequest(c, current, "dead-beef-123456")
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown model: "dead-beef-123456"`)
}

type toolsWithMacaroonsSuite struct {
	toolsCommonSuite
}

var _ = gc.Suite(&toolsWithMacaroonsSuite{})

func (s *toolsWithMacaroonsSuite) SetUpTest(c *gc.C) {
	s.macaroonAuthEnabled = true
	s.toolsCommonSuite.SetUpTest(c)
}

func (s *toolsWithMacaroonsSuite) TestWithNoBasicAuthReturnsDischargeRequiredError(c *gc.C) {
	resp := s.sendRequest(c, httpRequestParams{
		method: "POST",
		url:    s.toolsURI(c, ""),
	})

	charmResponse := s.assertResponse(c, resp, http.StatusUnauthorized)
	c.Assert(charmResponse.Error, gc.NotNil)
	c.Assert(charmResponse.Error.Message, gc.Equals, "verification failed: no macaroons")
	c.Assert(charmResponse.Error.Code, gc.Equals, params.CodeDischargeRequired)
	c.Assert(charmResponse.Error.Info, gc.NotNil)
	c.Assert(charmResponse.Error.Info.Macaroon, gc.NotNil)
}

func (s *toolsWithMacaroonsSuite) TestCanPostWithDischargedMacaroon(c *gc.C) {
	checkCount := 0
	s.DischargerLogin = func() string {
		checkCount++
		return s.userTag.Id()
	}
	resp := s.sendRequest(c, httpRequestParams{
		do:     s.doer(),
		method: "POST",
		url:    s.toolsURI(c, ""),
	})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
	c.Assert(checkCount, gc.Equals, 1)
}

// doer returns a Do function that can make a bakery request
// appropriate for a charms endpoint.
func (s *toolsWithMacaroonsSuite) doer() func(*http.Request) (*http.Response, error) {
	return bakeryDo(nil, bakeryGetError)
}
