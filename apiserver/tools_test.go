// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	coretools "github.com/juju/juju/tools"
)

type baseToolsSuite struct {
	apiserverBaseSuite
}

func (s *baseToolsSuite) toolsURL(query string) *url.URL {
	return s.modelToolsURL(s.Model.UUID(), query)
}

func (s *baseToolsSuite) modelToolsURL(model, query string) *url.URL {
	u := s.URL(fmt.Sprintf("/model/%s/tools", model), nil)
	u.RawQuery = query
	return u
}

func (s *baseToolsSuite) toolsURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.toolsURL(query).String()
}

func (s *baseToolsSuite) uploadRequest(c *gc.C, url, contentType string, content io.Reader) *http.Response {
	return s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         url,
		ContentType: contentType,
		Body:        content,
	})
}

func (s *baseToolsSuite) downloadRequest(c *gc.C, version version.Binary, uuid string) *http.Response {
	url := s.toolsURL("")
	if uuid == "" {
		url.Path = fmt.Sprintf("/tools/%s", version)
	} else {
		url.Path = fmt.Sprintf("/model/%s/tools/%s", uuid, version)
	}
	return apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: url.String()})
}

func (s *baseToolsSuite) assertUploadResponse(c *gc.C, resp *http.Response, agentTools *coretools.Tools) {
	toolsResponse := s.assertResponse(c, resp, http.StatusOK)
	c.Check(toolsResponse.Error, gc.IsNil)
	c.Check(toolsResponse.ToolsList, jc.DeepEquals, coretools.List{agentTools})
}

func (s *baseToolsSuite) assertJSONErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	toolsResponse := s.assertResponse(c, resp, expCode)
	c.Check(toolsResponse.ToolsList, gc.IsNil)
	c.Check(toolsResponse.Error, gc.NotNil)
	c.Check(toolsResponse.Error.Message, gc.Matches, expError)
}

func (s *baseToolsSuite) assertPlainErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	body := apitesting.AssertResponse(c, resp, expCode, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Matches, expError+"\n")
}

func (s *baseToolsSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.ToolsResult {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var toolsResponse params.ToolsResult
	err := json.Unmarshal(body, &toolsResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	return toolsResponse
}

func (s *baseToolsSuite) storeFakeTools(c *gc.C, st *state.State, content string, metadata binarystorage.Metadata) *coretools.Tools {
	storage, err := st.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	err = storage.Add(strings.NewReader(content), metadata)
	c.Assert(err, jc.ErrorIsNil)
	return &coretools.Tools{
		Version: version.MustParseBinary(metadata.Version),
		Size:    metadata.Size,
		SHA256:  metadata.SHA256,
	}
}

func (s *baseToolsSuite) getToolsFromStorage(c *gc.C, st *state.State, vers string) (binarystorage.Metadata, []byte) {
	storage, err := st.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	metadata, r, err := storage.Open(vers)
	c.Assert(err, jc.ErrorIsNil)
	data, err := ioutil.ReadAll(r)
	r.Close()
	c.Assert(err, jc.ErrorIsNil)
	return metadata, data
}

func (s *baseToolsSuite) getToolsMetadataFromStorage(c *gc.C, st *state.State) []binarystorage.Metadata {
	storage, err := st.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	metadata, err := storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	return metadata
}

func (s *baseToolsSuite) testDownload(c *gc.C, tools *coretools.Tools, uuid string) []byte {
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

type toolsSuite struct {
	baseToolsSuite
}

var _ = gc.Suite(&toolsSuite{})

func (s *toolsSuite) TestToolsUploadedSecurely(c *gc.C) {
	url := s.toolsURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "PUT",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *toolsSuite) TestRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.toolsURI("")})
	s.assertPlainErrorResponse(c, resp, http.StatusUnauthorized, "authentication failed: no credentials provided")
}

func (s *toolsSuite) TestRequiresPOST(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.toolsURI("")})
	s.assertJSONErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)
}

func (s *toolsSuite) TestAuthRejectsNonsUser(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:      machine.Tag().String(),
		Password: password,
		Method:   "POST",
		URL:      s.toolsURI(""),
		Nonce:    "fake_nonce",
	})
	s.assertPlainErrorResponse(
		c, resp, http.StatusForbidden,
		"authorization failed: permission denied",
	)

	// Now try a user login.
	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.toolsURI("")})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestAuthRejectsUserWithoutPermission(c *gc.C) {
	u := s.Factory.MakeUser(c, &factory.UserParams{
		Name:     "oryx",
		Password: "gardener",
		Access:   permission.WriteAccess,
	})

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:      u.Tag().String(),
		Password: "gardener",
		Method:   "POST",
		URL:      s.toolsURI(""),
		Nonce:    "fake_nonce",
	})
	s.assertPlainErrorResponse(
		c, resp, http.StatusForbidden,
		"authorization failed: permission denied",
	)
}

func (s *toolsSuite) TestUploadRequiresVersion(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.toolsURI("")})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
}

func (s *toolsSuite) TestUploadFailsWithNoTools(c *gc.C) {
	var empty bytes.Buffer
	resp := s.uploadRequest(c, s.toolsURI("?binaryVersion=1.18.0-ubuntu-amd64"), "application/x-tar-gz", &empty)
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "no agent binaries uploaded")
}

func (s *toolsSuite) TestUploadFailsWithInvalidContentType(c *gc.C) {
	var empty bytes.Buffer
	// Now try with the default Content-Type.
	resp := s.uploadRequest(c, s.toolsURI("?binaryVersion=1.18.0-ubuntu-amd64"), "application/octet-stream", &empty)
	s.assertJSONErrorResponse(
		c, resp, http.StatusBadRequest, "expected Content-Type: application/x-tar-gz, got: application/octet-stream")
}

func (s *toolsSuite) setupToolsForUpload(c *gc.C) (coretools.List, version.Binary, []byte) {
	localStorage := c.MkDir()
	vers := version.MustParseBinary("1.9.0-ubuntu-amd64")
	versionStrings := []string{vers.String()}
	expectedTools := toolstesting.MakeToolsWithCheckSum(c, localStorage, "released", versionStrings)
	toolsFile := envtools.StorageName(vers, "released")
	toolsContent, err := ioutil.ReadFile(filepath.Join(localStorage, toolsFile))
	c.Assert(err, jc.ErrorIsNil)
	return expectedTools, vers, toolsContent
}

func (s *toolsSuite) TestUpload(c *gc.C) {
	// Make some fake tools.
	expectedTools, v, toolsContent := s.setupToolsForUpload(c)
	vers := v.String()

	// Now try uploading them.
	resp := s.uploadRequest(
		c, s.toolsURI("?binaryVersion="+vers),
		"application/x-tar-gz",
		bytes.NewReader(toolsContent),
	)

	// Check the response.
	expectedTools[0].URL = s.toolsURL("").String() + "/" + vers
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	metadata, uploadedData := s.getToolsFromStorage(c, s.State, vers)
	c.Assert(uploadedData, gc.DeepEquals, toolsContent)
	allMetadata := s.getToolsMetadataFromStorage(c, s.State)
	c.Assert(allMetadata, jc.DeepEquals, []binarystorage.Metadata{metadata})
}

func (s *toolsSuite) TestMigrateTools(c *gc.C) {
	// Make some fake tools.
	expectedTools, v, toolsContent := s.setupToolsForUpload(c)
	vers := v.String()

	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()
	importedModel, err := newSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = importedModel.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	// Now try uploading them.
	uri := s.URL("/migrate/tools", url.Values{"binaryVersion": {vers}})
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         uri.String(),
		ContentType: "application/x-tar-gz",
		Body:        bytes.NewReader(toolsContent),
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: importedModel.UUID(),
		},
	})

	// Check the response.
	expectedTools[0].URL = s.modelToolsURL(s.State.ControllerModelUUID(), "").String() + "/" + vers
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	metadata, uploadedData := s.getToolsFromStorage(c, newSt, vers)
	c.Assert(uploadedData, gc.DeepEquals, toolsContent)
	allMetadata := s.getToolsMetadataFromStorage(c, newSt)
	c.Assert(allMetadata, jc.DeepEquals, []binarystorage.Metadata{metadata})
}

func (s *toolsSuite) TestMigrateToolsNotMigrating(c *gc.C) {
	// Make some fake tools.
	_, v, toolsContent := s.setupToolsForUpload(c)
	vers := v.String()

	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	uri := s.URL("/migrate/tools", url.Values{"binaryVersion": {vers}})
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         uri.String(),
		ContentType: "application/x-tar-gz",
		Body:        bytes.NewReader(toolsContent),
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: newSt.ModelUUID(),
		},
	})

	// Now try uploading them.
	s.assertJSONErrorResponse(
		c, resp, http.StatusBadRequest,
		`model migration mode is "" instead of "importing"`,
	)
}

func (s *toolsSuite) TestMigrateToolsUnauth(c *gc.C) {
	// Try uploading as a non controller admin.
	url := s.URL("/migrate/tools", nil).String()
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "hunter2"})
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "POST",
		URL:      url,
		Tag:      user.Tag().String(),
		Password: "hunter2",
	})
	s.assertPlainErrorResponse(
		c, resp, http.StatusForbidden,
		"authorization failed: user .* is not a controller admin",
	)
}

func (s *toolsSuite) TestBlockUpload(c *gc.C) {
	// Make some fake tools.
	_, v, toolsContent := s.setupToolsForUpload(c)
	vers := v.String()

	// Block all changes.
	err := s.State.SwitchBlockOn(state.ChangeBlock, "TestUpload")
	c.Assert(err, jc.ErrorIsNil)

	// Now try uploading them.
	resp := s.uploadRequest(
		c, s.toolsURI("?binaryVersion="+vers),
		"application/x-tar-gz",
		bytes.NewReader(toolsContent),
	)
	toolsResponse := s.assertResponse(c, resp, http.StatusBadRequest)
	c.Assert(toolsResponse.Error, jc.Satisfies, params.IsCodeOperationBlocked)
	c.Assert(errors.Cause(toolsResponse.Error), gc.DeepEquals, &params.Error{
		Message: "TestUpload",
		Code:    "operation is blocked",
	})

	// Check the contents.
	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	_, _, err = storage.Open(vers)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *toolsSuite) TestUploadAllowsTopLevelPath(c *gc.C) {
	// Backwards compatibility check, that we can upload tools to
	// https://host:port/tools
	expectedTools, vers, toolsContent := s.setupToolsForUpload(c)
	url := s.toolsURL("binaryVersion=" + vers.String())
	url.Path = "/tools"
	resp := s.uploadRequest(c, url.String(), "application/x-tar-gz", bytes.NewReader(toolsContent))
	expectedTools[0].URL = s.modelToolsURL(s.State.ControllerModelUUID(), "").String() + "/" + vers.String()
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadAllowsModelUUIDPath(c *gc.C) {
	// Check that we can upload tools to https://host:port/ModelUUID/tools
	expectedTools, vers, toolsContent := s.setupToolsForUpload(c)
	url := s.toolsURL("binaryVersion=" + vers.String())
	resp := s.uploadRequest(c, url.String(), "application/x-tar-gz", bytes.NewReader(toolsContent))
	// Check the response.
	expectedTools[0].URL = s.toolsURL("").String() + "/" + vers.String()
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadAllowsOtherModelUUIDPath(c *gc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	// Check that we can upload tools to https://host:port/ModelUUID/tools
	expectedTools, vers, toolsContent := s.setupToolsForUpload(c)
	url := s.modelToolsURL(newSt.ModelUUID(), "binaryVersion="+vers.String())
	resp := s.uploadRequest(c, url.String(), "application/x-tar-gz", bytes.NewReader(toolsContent))

	// Check the response.
	expectedTools[0].URL = s.modelToolsURL(newSt.ModelUUID(), "").String() + "/" + vers.String()
	s.assertUploadResponse(c, resp, expectedTools[0])
}

func (s *toolsSuite) TestUploadConvertsSeries(c *gc.C) {
	// Make some fake tools.
	expectedTools, v, toolsContent := s.setupToolsForUpload(c)
	vCopy := v
	vCopy.Release = "bionic"
	vers := v.String()
	// Now try uploading them. The tools will be cloned for
	// each additional series specified.
	params := "?binaryVersion=" + vCopy.String()
	resp := s.uploadRequest(c, s.toolsURI(params), "application/x-tar-gz", bytes.NewReader(toolsContent))
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)

	// Check the response.
	expectedTools[0].URL = s.toolsURL("").String() + "/" + vers
	s.assertUploadResponse(c, resp, expectedTools[0])

	// Check the contents.
	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	_, r, err := storage.Open(v.String())
	c.Assert(err, jc.ErrorIsNil)
	uploadedData, err := ioutil.ReadAll(r)
	r.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadedData, gc.DeepEquals, toolsContent)

	// ensure the series *isn't* there.
	v.Release = "bionic"
	_, err = storage.Metadata(v.String())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *toolsSuite) TestDownloadModelUUIDPath(c *gc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", binarystorage.Metadata{
		Version: testing.CurrentVersion().String(),
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, s.State.ModelUUID())
}

// TODO(juju4) - remove
func (s *toolsSuite) TestDownloadOldAgentNewRequest(c *gc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", binarystorage.Metadata{
		Version: "2.8.9-focal-amd64",
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	resp := s.downloadRequest(c, version.MustParseBinary("2.8.9-ubuntu-amd64"), s.State.ModelUUID())
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, int(tools.Size))

	hash := sha256.New()
	hash.Write(data)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), gc.Equals, tools.SHA256)
}

// TODO(juju4) - remove
func (s *toolsSuite) TestDownloadAgentOldRequest(c *gc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", binarystorage.Metadata{
		Version: "2.8.9-ubuntu-amd64",
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	resp := s.downloadRequest(c, version.MustParseBinary("2.8.9-focal-amd64"), s.State.ModelUUID())
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, int(tools.Size))

	hash := sha256.New()
	hash.Write(data)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), gc.Equals, tools.SHA256)
}

// TODO(juju4) - remove
func (s *toolsSuite) TestDownloadSeriesAgentOldRequest(c *gc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", binarystorage.Metadata{
		Version: "2.8.9-bionic-amd64",
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	resp := s.downloadRequest(c, version.MustParseBinary("2.8.9-focal-amd64"), s.State.ModelUUID())
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, int(tools.Size))

	hash := sha256.New()
	hash.Write(data)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), gc.Equals, tools.SHA256)
}

func (s *toolsSuite) TestDownloadOtherModelUUIDPath(c *gc.C) {
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()

	tools := s.storeFakeTools(c, newSt, "abc", binarystorage.Metadata{
		Version: testing.CurrentVersion().String(),
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, newSt.ModelUUID())
}

func (s *toolsSuite) TestDownloadTopLevelPath(c *gc.C) {
	tools := s.storeFakeTools(c, s.State, "abc", binarystorage.Metadata{
		Version: testing.CurrentVersion().String(),
		Size:    3,
		SHA256:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})
	s.testDownload(c, tools, "")
}

func (s *toolsSuite) TestDownloadMissingConcurrent(c *gc.C) {
	closer, testStorage, _ := envtesting.CreateLocalTestStorage(c)
	defer closer.Close()

	var mut sync.Mutex
	resolutions := 0
	envtools.RegisterToolsDataSourceFunc("local storage", func(environs.Environ) (simplestreams.DataSource, error) {
		// Add some delay to make sure all goroutines are waiting.
		time.Sleep(10 * time.Millisecond)
		mut.Lock()
		defer mut.Unlock()
		resolutions++
		return storage.NewStorageSimpleStreamsDataSource("test datasource", testStorage, "tools", simplestreams.CUSTOM_CLOUD_DATA, false), nil
	})
	defer envtools.UnregisterToolsDataSourceFunc("local storage")

	toolsBinaries := []version.Binary{
		version.MustParseBinary("2.9.98-ubuntu-amd64"),
		version.MustParseBinary("2.9.99-ubuntu-amd64"),
	}
	stream := "released"
	tools, err := envtesting.UploadFakeToolsVersions(testStorage, stream, stream, toolsBinaries...)
	c.Assert(err, jc.ErrorIsNil)

	var wg sync.WaitGroup
	const n = 8
	wg.Add(n)
	for i := 0; i < n; i++ {
		tool := tools[i%len(toolsBinaries)]
		go func() {
			defer wg.Done()
			s.testDownload(c, tool, s.State.ModelUUID())
		}()
	}
	wg.Wait()

	c.Assert(resolutions, gc.Equals, len(toolsBinaries))
}

type caasToolsSuite struct {
	baseToolsSuite
}

var _ = gc.Suite(&caasToolsSuite{})

func (s *caasToolsSuite) SetUpTest(c *gc.C) {
	s.ControllerModelType = state.ModelTypeCAAS
	s.baseToolsSuite.SetUpTest(c)
}

func (s *caasToolsSuite) TestToolDownloadNotSharedCAASController(c *gc.C) {
	closer, testStorage, _ := envtesting.CreateLocalTestStorage(c)
	defer closer.Close()

	const n = 8
	states := []*state.State{}
	for i := 0; i < n; i++ {
		testState := s.Factory.MakeModel(c, nil)
		defer testState.Close()
		states = append(states, testState)
	}

	var mut sync.Mutex
	resolutions := 0
	envtools.RegisterToolsDataSourceFunc("local storage", func(environs.Environ) (simplestreams.DataSource, error) {
		// Add some delay to make sure all goroutines are waiting.
		time.Sleep(10 * time.Millisecond)
		mut.Lock()
		defer mut.Unlock()
		resolutions++
		return storage.NewStorageSimpleStreamsDataSource("test datasource", testStorage, "tools", simplestreams.CUSTOM_CLOUD_DATA, false), nil
	})
	defer envtools.UnregisterToolsDataSourceFunc("local storage")

	tool := version.MustParseBinary("2.9.99-ubuntu-amd64")
	stream := "released"
	tools, err := envtesting.UploadFakeToolsVersions(testStorage, stream, stream, tool)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools, gc.HasLen, 1)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			s.testDownload(c, tools[0], states[i].ModelUUID())
		}()
	}
	wg.Wait()

	c.Assert(resolutions, gc.Equals, n)
}
