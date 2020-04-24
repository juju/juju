// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	apiauthentication "github.com/juju/juju/api/authentication"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/params"
	servertesting "github.com/juju/juju/apiserver/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type toolsCommonSuite struct {
	baseURL   *url.URL
	modelUUID string
}

func (s *toolsCommonSuite) toolsURL(query string) *url.URL {
	return s.modelToolsURL(s.modelUUID, query)
}

func (s *toolsCommonSuite) toolsURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.toolsURL(query).String()
}

func (s *toolsCommonSuite) modelToolsURL(model, query string) *url.URL {
	u := s.URL(fmt.Sprintf("/model/%s/tools", model), nil)
	u.RawQuery = query
	return u
}

func (s *toolsCommonSuite) assertJSONErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	toolsResponse := assertResponse(c, resp, expCode)
	c.Check(toolsResponse.ToolsList, gc.IsNil)
	c.Check(toolsResponse.Error, gc.NotNil)
	c.Check(toolsResponse.Error.Message, gc.Matches, expError)
}

// URL returns a URL for this server with the given path and
// query parameters. The URL scheme will be "https".
func (s *toolsCommonSuite) URL(path string, queryParams url.Values) *url.URL {
	url := *s.baseURL
	url.Path = path
	url.RawQuery = queryParams.Encode()
	return &url
}

type toolsDownloadSuite struct {
	toolsCommonSuite
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&toolsDownloadSuite{})

func (s *toolsDownloadSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	apiInfo := s.APIInfo(c)
	baseURL, err := url.Parse(fmt.Sprintf("https://%s/", apiInfo.Addrs[0]))
	c.Assert(err, jc.ErrorIsNil)
	s.baseURL = baseURL
	s.modelUUID = s.Model.UUID()
}

func (s *toolsDownloadSuite) TestDownloadFetchesAndCaches(c *gc.C) {
	// The tools are not in binarystorage, so the download request causes
	// the API server to search for the tools in simplestreams, fetch
	// them, and then cache them in binarystorage.
	vers := version.MustParseBinary("1.23.0-trusty-amd64")
	stor := s.DefaultToolsStorage
	envtesting.RemoveTools(c, stor, "released")
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", vers)[0]
	data := s.testDownload(c, tools, "")

	metadata, cachedData := s.getToolsFromStorage(c, s.State, tools.Version.String())
	c.Assert(metadata.Size, gc.Equals, tools.Size)
	c.Assert(metadata.SHA256, gc.Equals, tools.SHA256)
	c.Assert(string(cachedData), gc.Equals, string(data))
}

func (s *toolsDownloadSuite) TestDownloadFetchesAndVerifiesSize(c *gc.C) {
	// Upload fake tools, then upload over the top so the SHA256 hash does not match.
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	stor := s.DefaultToolsStorage
	envtesting.RemoveTools(c, stor, "released")
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", current)[0]
	err := stor.Put(envtools.StorageName(tools.Version, "released"), strings.NewReader("!"), 1)
	c.Assert(err, jc.ErrorIsNil)

	resp := s.downloadRequest(c, tools.Version, "")
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "error fetching agent binaries: size mismatch for .*")
	s.assertToolsNotStored(c, tools.Version.String())
}

func (s *toolsDownloadSuite) TestDownloadFetchesAndVerifiesHash(c *gc.C) {
	// Upload fake tools, then upload over the top so the SHA256 hash does not match.
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	stor := s.DefaultToolsStorage
	envtesting.RemoveTools(c, stor, "released")
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", current)[0]
	sameSize := strings.Repeat("!", int(tools.Size))
	err := stor.Put(envtools.StorageName(tools.Version, "released"), strings.NewReader(sameSize), tools.Size)
	c.Assert(err, jc.ErrorIsNil)

	resp := s.downloadRequest(c, tools.Version, "")
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "error fetching agent binaries: hash mismatch for .*")
	s.assertToolsNotStored(c, tools.Version.String())
}

func (s *toolsDownloadSuite) testDownload(c *gc.C, tools *coretools.Tools, uuid string) []byte {
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

func (s *toolsDownloadSuite) downloadRequest(c *gc.C, version version.Binary, uuid string) *http.Response {
	url := s.toolsURL("")
	if uuid == "" {
		url.Path = fmt.Sprintf("/tools/%s", version)
	} else {
		url.Path = fmt.Sprintf("/model/%s/tools/%s", uuid, version)
	}
	return servertesting.SendHTTPRequest(c, servertesting.HTTPRequestParams{Method: "GET", URL: url.String()})
}

func (s *toolsDownloadSuite) getToolsFromStorage(c *gc.C, st *state.State, vers string) (binarystorage.Metadata, []byte) {
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

func (s *toolsDownloadSuite) assertToolsNotStored(c *gc.C, vers string) {
	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	_, err = storage.Metadata(vers)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func assertResponse(c *gc.C, resp *http.Response, expStatus int) params.ToolsResult {
	body := servertesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var toolsResponse params.ToolsResult
	err := json.Unmarshal(body, &toolsResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	return toolsResponse
}

type toolsWithMacaroonsSuite struct {
	toolsCommonSuite
	apitesting.MacaroonSuite
	userTag names.Tag
}

var _ = gc.Suite(&toolsWithMacaroonsSuite{})

func (s *toolsWithMacaroonsSuite) SetUpTest(c *gc.C) {
	s.MacaroonSuite.SetUpTest(c)
	s.userTag = names.NewUserTag("bob@authhttpsuite")
	s.AddModelUser(c, s.userTag.Id())
	apiInfo := s.APIInfo(c)
	baseURL, err := url.Parse(fmt.Sprintf("https://%s/", apiInfo.Addrs[0]))
	c.Assert(err, jc.ErrorIsNil)
	s.baseURL = baseURL
	s.modelUUID = s.Model.UUID()
}

func (s *toolsWithMacaroonsSuite) TestWithNoBasicAuthReturnsDischargeRequiredError(c *gc.C) {
	resp := servertesting.SendHTTPRequest(c, servertesting.HTTPRequestParams{
		Method: "POST",
		URL:    s.toolsURI(""),
	})

	charmResponse := assertResponse(c, resp, http.StatusUnauthorized)
	c.Assert(charmResponse.Error, gc.NotNil)
	c.Assert(charmResponse.Error.Message, gc.Equals, "macaroon discharge required: authentication required")
	c.Assert(charmResponse.Error.Code, gc.Equals, params.CodeDischargeRequired)
	c.Assert(charmResponse.Error.Info, gc.NotNil)
	c.Assert(charmResponse.Error.Info["bakery-macaroon"], gc.NotNil)
}

func (s *toolsWithMacaroonsSuite) TestCanPostWithDischargedMacaroon(c *gc.C) {
	checkCount := 0
	s.DischargerLogin = func() string {
		checkCount++
		return s.userTag.Id()
	}
	resp := servertesting.SendHTTPRequest(c, servertesting.HTTPRequestParams{
		Do:     s.doer(),
		Method: "POST",
		URL:    s.toolsURI(""),
	})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
	c.Assert(checkCount, gc.Equals, 1)
}

func (s *toolsWithMacaroonsSuite) TestCanPostWithLocalLogin(c *gc.C) {
	// Create a new local user that we can log in as
	// using macaroon authentication.
	const password = "hunter2"
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: password})

	// Install a "web-page" visitor that deals with the interaction
	// method that Juju controllers support for authenticating local
	// users. Note: the use of httpbakery.NewMultiVisitor is necessary
	// to trigger httpbakery to query the authentication methods and
	// bypass browser authentication.
	var prompted bool
	jar := apitesting.NewClearableCookieJar()
	client := utils.GetNonValidatingHTTPClient()
	client.Jar = jar
	bakeryClient := httpbakery.NewClient()
	bakeryClient.Client = client
	bakeryClient.AddInteractor(apiauthentication.NewInteractor(
		user.UserTag().Id(),
		func(username string) (string, error) {
			c.Assert(username, gc.Equals, user.UserTag().Id())
			prompted = true
			return password, nil
		},
	))
	bakeryDo := func(req *http.Request) (*http.Response, error) {
		c.Logf("req.URL: %#v", req.URL)
		return bakeryClient.DoWithCustomError(req, bakeryGetError)
	}

	resp := servertesting.SendHTTPRequest(c, servertesting.HTTPRequestParams{
		Method:   "POST",
		URL:      s.toolsURI(""),
		Tag:      user.UserTag().String(),
		Password: "", // no password forces macaroon usage
		Do:       bakeryDo,
	})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
	c.Assert(prompted, jc.IsTrue)
}

// doer returns a Do function that can make a bakery request
// appropriate for a charms endpoint.
func (s *toolsWithMacaroonsSuite) doer() func(*http.Request) (*http.Response, error) {
	return bakeryDo(nil, bakeryGetError)
}

// bakeryDo provides a function suitable for using in HTTPRequestParams.Do
// that will use the given http client (or utils.GetNonValidatingHTTPClient()
// if client is nil) and use the given getBakeryError function
// to translate errors in responses.
func bakeryDo(client *http.Client, getBakeryError func(*http.Response) error) func(*http.Request) (*http.Response, error) {
	bclient := httpbakery.NewClient()
	if client != nil {
		bclient.Client = client
	} else {
		// Configure the default client to skip verification/
		tlsConfig := utils.SecureTLSConfig()
		tlsConfig.InsecureSkipVerify = true
		bclient.Client.Transport = utils.NewHttpTLSTransport(tlsConfig)
	}
	return func(req *http.Request) (*http.Response, error) {
		return bclient.DoWithCustomError(req, getBakeryError)
	}
}

// bakeryGetError implements a getError function
// appropriate for passing to httpbakery.Client.DoWithBodyAndCustomError
// for any endpoint that returns the error in a top level Error field.
func bakeryGetError(resp *http.Response) error {
	if resp.StatusCode != http.StatusUnauthorized {
		return nil
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Annotatef(err, "cannot read body")
	}
	var errResp params.ErrorResult
	if err := json.Unmarshal(data, &errResp); err != nil {
		return errors.Annotatef(err, "cannot unmarshal body")
	}
	if errResp.Error == nil {
		return errors.New("no error found in error response body")
	}
	if errResp.Error.Code != params.CodeDischargeRequired {
		return errResp.Error
	}
	if errResp.Error.Info == nil {
		return errors.Annotatef(err, "no error info found in discharge-required response error")
	}
	// It's a discharge-required error, so make an appropriate httpbakery
	// error from it.
	var info params.DischargeRequiredErrorInfo
	if errUnmarshal := errResp.Error.UnmarshalInfo(&info); errUnmarshal != nil {
		return errors.Annotatef(err, "unable to extract macaroon details from discharge-required response error")
	}

	mac := info.BakeryMacaroon
	if mac == nil {
		var err error
		mac, err = bakery.NewLegacyMacaroon(info.Macaroon)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return &httpbakery.Error{
		Message: errResp.Error.Message,
		Code:    httpbakery.ErrDischargeRequired,
		Info: &httpbakery.ErrorInfo{
			Macaroon:     mac,
			MacaroonPath: info.MacaroonPath,
		},
	}
}
