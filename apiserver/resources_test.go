// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmresources"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type resourcesSuite struct {
	userAuthHttpSuite
	commontesting.BlockHelper
	baseResourcesSuite
	unitTag      names.UnitTag
	unitPassword string
}

var _ = gc.Suite(&resourcesSuite{})

func (s *resourcesSuite) SetUpSuite(c *gc.C) {
	s.userAuthHttpSuite.SetUpSuite(c)
	s.baseResourcesSuite.SetUpSuite(c)
	s.archiveContentType = "application/x-tar-gz"
}

func (s *resourcesSuite) SetUpTest(c *gc.C) {
	s.userAuthHttpSuite.SetUpTest(c)
	s.baseResourcesSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
	unit, password := s.Factory.MakeUnitReturningPassword(c, &factory.UnitParams{})
	s.unitTag = unit.UnitTag()
	s.unitPassword = password
}

func (s *resourcesSuite) TearDownSuite(c *gc.C) {
	s.baseResourcesSuite.TearDownSuite(c)
	s.userAuthHttpSuite.TearDownSuite(c)
}

func (s *resourcesSuite) TearDownTest(c *gc.C) {
	s.baseResourcesSuite.TearDownTest(c)
	s.userAuthHttpSuite.TearDownTest(c)
}

func (s *resourcesSuite) TestResourcesUploadedSecurely(c *gc.C) {
	info := s.APIInfo(c)
	uri := "http://" + info.Addrs[0] + "/resources"
	_, err := s.sendRequest(c, "", "", "PUT", uri, "", nil)
	c.Assert(err, gc.ErrorMatches, `.*malformed HTTP response.*`)
}

func (s *resourcesSuite) TestRequiresAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "GET", s.resourcesURI(c, "path", ""), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")
	resp, err = s.sendRequest(c, "", "", "POST", s.resourcesURI(c, "", "path=foo?revision=1"), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")
}

func (s *resourcesSuite) TestRequiresPOST(c *gc.C) {
	resp, err := s.authRequest(c, "PUT", s.resourcesURI(c, "", "path=foo"), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)
}

func (s *resourcesSuite) TestAuthRequiresUser(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	resp, err := s.sendRequest(c, machine.Tag().String(), password, "POST", s.resourcesURI(c, "", "path=foo"), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")

	// Now try a user login.
	resp, err = s.authRequest(c, "POST", s.resourcesURI(c, "", "path=foo"), "application/octet-stream", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "revision is required to upload resources")
}

func (s *resourcesSuite) TestUploadRequiresRevision(c *gc.C) {
	resp, err := s.authRequest(c, "POST", s.resourcesURI(c, "", "path=foo"), "application/octet-stream", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "revision is required to upload resources")
}

func (s *resourcesSuite) TestUploadFailsWithNoResources(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "resource")
	c.Assert(err, jc.ErrorIsNil)

	resp, err := s.uploadRequest(c, s.resourcesURI(c, "", "path=foo&revision=v1.2"), false, tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "no resources uploaded")
}

func (s *resourcesSuite) TestUploadFailsWithInvalidContentType(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "resource")
	c.Assert(err, jc.ErrorIsNil)

	// Now try with the default Content-Type.
	resp, err := s.uploadRequest(c, s.resourcesURI(c, "", "path=foo&revision=v1.2"), true, tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(
		c, resp, http.StatusBadRequest, "expected Content-Type: application/octet-stream, got: application/x-tar-gz")
}

func (s *resourcesSuite) setupResourceForUpload(c *gc.C) (*params.ResourceMetadata, string) {
	localStorage := c.MkDir()
	path := filepath.Join(localStorage, "test-resource.dat")
	data := "this is a resource"
	err := ioutil.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	metadata := &params.ResourceMetadata{
		ResourcePath: "/blob/test-resource/1.2.3",
	}
	metadata.Size, metadata.SHA384 = sha384sum(c, path)
	return metadata, path
}

func sha384sum(c *gc.C, path string) (int64, string) {
	f, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)

	defer f.Close()
	hash := sha512.New384()
	size, err := io.Copy(hash, f)
	c.Assert(err, jc.ErrorIsNil)
	digest := hex.EncodeToString(hash.Sum(nil))
	return size, digest
}

func (s *resourcesSuite) TestUpload(c *gc.C) {
	// Make a fake resource.
	expectedMetadata, resPath := s.setupResourceForUpload(c)
	expectedMetadata.ResourcePath = "/zip/u/fred/c/test/s/trusty/test-resource/1.2.3"
	// Now try uploading them.
	resourceParams := "revision=1.2.3&series=trusty&stream=test&type=zip&user=fred"
	resp, err := s.uploadRequest(
		c, s.resourcesURI(c, "", "path=test-resource&"+resourceParams), false, resPath)
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		getBlockForTypeCall,
		resourceManagerCall,
		resourcePutCall,
	}
	s.assertCalls(c, expectedCalls)

	// Check the response.
	expectedMetadata.URL = fmt.Sprintf("%s/environment/%s/resources/test-resource?"+resourceParams, s.baseURL(c), s.State.EnvironUUID())
	c.Assert(expectedMetadata.Created.Unix(), gc.Not(gc.Equals), 0)
	s.assertUploadResponse(c, resp, expectedMetadata)

	// Check the contents.
	uploadedData := s.resourcesData["/zip/u/fred/c/test/s/trusty/test-resource/1.2.3"]
	expectedData, err := ioutil.ReadFile(resPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadedData, gc.DeepEquals, expectedData)

	// Can it be downloaded again ie is the URL correct.
	metadata := charmresources.Resource{
		Path:       "/zip/u/fred/c/test/s/trusty/test-resource/1.2.3",
		Size:       expectedMetadata.Size,
		SHA384Hash: expectedMetadata.SHA384,
	}
	s.testDownload(c, expectedMetadata.URL, metadata)
}

func (s *resourcesSuite) TestBlockUpload(c *gc.C) {
	// Make a fake resource.
	_, resPath := s.setupResourceForUpload(c)
	// Block all changes.
	s.blockAllChanges(c, "TestUpload")
	// Now try uploading them.
	resp, err := s.uploadRequest(
		c, s.resourcesURI(c, "", "path=test-resource&revision=1.2.3"), false, resPath)
	c.Check(err, jc.ErrorIsNil)
	problem := s.assertErrorResponse(c, resp, http.StatusBadRequest, "TestUpload")
	s.AssertBlocked(c, problem, "TestUpload")

	expectedCalls := []string{
		getBlockForTypeCall,
	}
	s.assertCalls(c, expectedCalls)

	// Check the contents.
	_, ok := s.resourcesData["/blob/test-resource/1.2.3"]
	c.Assert(ok, jc.IsFalse)
}

func (s *resourcesSuite) TestUploadAllowsOtherEnvUUIDPath(c *gc.C) {
	envState := s.setupOtherEnvironment(c)
	// Make a fake resource.
	expectedMetadata, resPath := s.setupResourceForUpload(c)
	url := s.resourcesURL(c, "", "path=test-resource&revision=1.2.3")
	url.Path = fmt.Sprintf("/environment/%s/resources", envState.EnvironUUID())
	resp, err := s.uploadRequest(c, url.String(), false, resPath)
	c.Assert(err, jc.ErrorIsNil)

	// Check the response.
	expectedMetadata.URL = fmt.Sprintf(
		"%s/environment/%s/resources/test-resource?revision=1.2.3", s.baseURL(c), envState.EnvironUUID())
	s.assertUploadResponse(c, resp, expectedMetadata)
}

func (s *resourcesSuite) TestUploadRejectsWrongEnvUUIDPath(c *gc.C) {
	// Check that we cannot access the resources at https://host:port/BADENVUUID/resources
	url := s.resourcesURL(c, "", "path=test-resource&revision=1.2.3")
	url.Path = "/environment/dead-beef-123456/resources"
	resp, err := s.authRequest(c, "POST", url.String(), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
}

func (s *resourcesSuite) TestDownload(c *gc.C) {
	metadata := charmresources.Resource{
		Path: "/blob/test-resource/1.2.3",
		Size: 3,
	}
	hash := sha512.New384()
	hash.Write([]byte("abc"))
	metadata.SHA384Hash = hex.EncodeToString(hash.Sum(nil))

	s.storeFakeResource(c, "abc", metadata)
	url := s.resourcesURL(c, "test-resource", "revision=1.2.3")
	s.testDownload(c, url.String(), metadata)

	expectedCalls := []string{
		resourceManagerCall,
		resourceGetCall,
	}
	s.assertCalls(c, expectedCalls)

}

func (s *resourcesSuite) TestDownloadOtherEnvUUIDPath(c *gc.C) {
	envState := s.Factory.MakeEnvironment(c, &factory.EnvParams{
		ConfigAttrs: map[string]interface{}{
			"state-server": false,
		},
		Prepare: true,
	})
	s.AddCleanup(func(*gc.C) { envState.Close() })
	unitFactory := factory.NewFactory(envState)
	unit, password := unitFactory.MakeUnitReturningPassword(c, &factory.UnitParams{})
	s.unitTag = unit.UnitTag()
	s.unitPassword = password
	metadata := charmresources.Resource{
		Path: "/blob/test-resource/1.2.3",
		Size: 3,
	}
	hash := sha512.New384()
	hash.Write([]byte("abc"))
	metadata.SHA384Hash = hex.EncodeToString(hash.Sum(nil))

	s.storeFakeResource(c, "abc", metadata)
	url := s.resourcesURL(c, "test-resource", "revision=1.2.3")
	url.Path = fmt.Sprintf("/environment/%s/resources/test-resource", envState.EnvironUUID())
	s.testDownload(c, url.String(), metadata)
}

func (s *resourcesSuite) testDownload(c *gc.C, url string, metadata charmresources.Resource) []byte {
	resp, err := s.downloadRequest(c, url)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.HasLen, int(metadata.Size))

	hash := sha512.New384()
	hash.Write(data)
	c.Assert(fmt.Sprintf("%x", hash.Sum(nil)), gc.Equals, metadata.SHA384Hash)
	return data
}

func (s *resourcesSuite) TestDownloadRejectsWrongEnvUUIDPath(c *gc.C) {
	url := s.resourcesURL(c, "test-resource", "revision=1.2.3")
	url.Path = "/environment/dead-beef-123456/resources/test-resource"
	resp, err := s.downloadRequest(c, url.String())
	c.Assert(err, jc.ErrorIsNil)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`)
}

func (s *resourcesSuite) resourcesURL(c *gc.C, basePath, query string) *url.URL {
	uri := s.baseURL(c)
	if basePath != "" && basePath[0] != '/' {
		basePath = "/" + basePath
	}
	uri.Path = fmt.Sprintf("/environment/%s/resources%s", s.envUUID, basePath)
	uri.RawQuery = query
	return uri
}

func (s *resourcesSuite) resourcesURI(c *gc.C, basePath, query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.resourcesURL(c, basePath, query).String()
}

func (s *resourcesSuite) downloadRequest(c *gc.C, url string) (*http.Response, error) {
	return s.sendRequest(c, s.unitTag.String(), s.unitPassword, "GET", url, "", nil)
}

func (s *resourcesSuite) assertUploadResponse(c *gc.C, resp *http.Response, resource *params.ResourceMetadata) {
	body := assertResponse(c, resp, http.StatusOK, apihttp.CTypeJSON)
	resourceResult := jsonResourcesResponse(c, body)
	c.Check(resourceResult.Error, gc.IsNil)
	// Make the crested times match so we can compare.
	resource.Created = time.Now()
	resourceResult.Resource.Created = resource.Created
	c.Check(&resourceResult.Resource, gc.DeepEquals, resource)
}

func (s *resourcesSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := assertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *resourcesSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) error {
	body := assertResponse(c, resp, expCode, apihttp.CTypeJSON)
	err := jsonResourcesResponse(c, body).Error
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, expError)
	return err
}

func jsonResourcesResponse(c *gc.C, body []byte) (jsonResponse params.ResourceResult) {
	err := json.Unmarshal(body, &jsonResponse)
	c.Assert(err, jc.ErrorIsNil)
	return
}

// baseResourceSuite provides mocked state and resource storage.
type baseResourcesSuite struct {
	coretesting.BaseSuite

	state *mockState

	calls []string

	resourceManager *mockResourceManager
	resources       map[string]charmresources.Resource
	resourcesData   map[string][]byte
	blocks          map[state.BlockType]state.Block
}

func (s *baseResourcesSuite) SetUpTest(c *gc.C) {
	s.calls = []string{}
	s.state = s.constructState(c)

	s.resources = make(map[string]charmresources.Resource)
	s.resourcesData = make(map[string][]byte)
	s.resourceManager = s.constructResourceManager(c)
	st := s.constructState(c)
	s.PatchValue(apiserver.GetResourceState, func(*state.State) apiserver.ResourceState {
		return st
	})
}

func (s *baseResourcesSuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

const (
	resourceGetCall     = "resourceGet"
	resourcePutCall     = "resourcePut"
	resourceManagerCall = "resourceManager"
	getBlockForTypeCall = "getBlockForType"
)

func (s *baseResourcesSuite) constructState(c *gc.C) *mockState {
	s.blocks = make(map[state.BlockType]state.Block)
	return &mockState{
		resourceManager: func() charmresources.ResourceManager {
			s.calls = append(s.calls, resourceManagerCall)
			return s.resourceManager
		},
		getBlockForType: func(t state.BlockType) (state.Block, bool, error) {
			s.calls = append(s.calls, getBlockForTypeCall)
			val, found := s.blocks[t]
			return val, found, nil
		},
	}
}

func (s *baseResourcesSuite) storeFakeResource(c *gc.C, content string, metadata charmresources.Resource) {
	s.resources[metadata.Path] = metadata
	s.resourcesData[metadata.Path] = []byte(content)
}

func (s *baseResourcesSuite) constructResourceManager(c *gc.C) *mockResourceManager {
	return &mockResourceManager{
		resourcePut: func(metadata charmresources.Resource, rdr io.Reader) (charmresources.Resource, error) {
			s.calls = append(s.calls, resourcePutCall)
			sha384hash := sha512.New384()
			r := io.TeeReader(rdr, sha384hash)
			data, err := ioutil.ReadAll(r)
			c.Assert(err, jc.ErrorIsNil)
			s.resourcesData[metadata.Path] = data
			res := metadata
			res.Size = int64(len(data))
			res.SHA384Hash = hex.EncodeToString(sha384hash.Sum(nil))
			return res, nil
		},
		resourceGet: func(resourcePath string) ([]charmresources.ResourceReader, error) {
			s.calls = append(s.calls, resourceGetCall)
			data, ok := s.resourcesData[resourcePath]
			if !ok {
				return nil, errors.NotFoundf("resource at path %v", resourcePath)
			}
			hash := sha512.New384()
			hash.Write(data)
			metadata := charmresources.Resource{
				Path:       resourcePath,
				Size:       int64(len(data)),
				SHA384Hash: hex.EncodeToString(hash.Sum(nil)),
			}
			rdr := ioutil.NopCloser(bytes.NewReader(data))
			return []charmresources.ResourceReader{
				{rdr, metadata},
			}, nil
		},
	}
}

type mockResourceManager struct {
	charmresources.ResourceManager
	resourcePut func(metadata charmresources.Resource, rdr io.Reader) (charmresources.Resource, error)
	resourceGet func(resourcePath string) ([]charmresources.ResourceReader, error)
}

func (m *mockResourceManager) ResourceGet(resourcePath string) ([]charmresources.ResourceReader, error) {
	return m.resourceGet(resourcePath)
}

func (m *mockResourceManager) ResourcePut(metadata charmresources.Resource, rdr io.Reader) (charmresources.Resource, error) {
	return m.resourcePut(metadata, rdr)
}

type mockState struct {
	resourceManager func() charmresources.ResourceManager
	getBlockForType func(t state.BlockType) (state.Block, bool, error)
}

func (st *mockState) ResourceManager() charmresources.ResourceManager {
	return st.resourceManager()
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return st.getBlockForType(t)
}

func (st *mockState) EnvironUUID() string {
	return "uuid"
}

func (s *baseResourcesSuite) addBlock(c *gc.C, t state.BlockType, msg string) {
	s.blocks[t] = mockBlock{t: t, msg: msg}
}

func (s *baseResourcesSuite) blockAllChanges(c *gc.C, msg string) {
	s.addBlock(c, state.ChangeBlock, msg)
}

type mockBlock struct {
	state.Block
	t   state.BlockType
	msg string
}

func (b mockBlock) Type() state.BlockType {
	return b.t
}

func (b mockBlock) Message() string {
	return b.msg
}
