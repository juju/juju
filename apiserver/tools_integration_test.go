// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiauthentication "github.com/juju/juju/api/authentication"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/domain/user/service"
	"github.com/juju/juju/internal/auth"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing/factory"
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

func assertResponse(c *gc.C, resp *http.Response, expStatus int) params.ToolsResult {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var toolsResponse params.ToolsResult
	err := json.Unmarshal(body, &toolsResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	return toolsResponse
}

type toolsWithMacaroonsIntegrationSuite struct {
	toolsCommonSuite
	jujutesting.MacaroonSuite
	userTag names.Tag
}

var _ = gc.Suite(&toolsWithMacaroonsIntegrationSuite{})

func (s *toolsWithMacaroonsIntegrationSuite) SetUpTest(c *gc.C) {
	s.MacaroonSuite.SetUpTest(c)
	s.userTag = names.NewUserTag("bob@authhttpsuite")
	s.AddModelUser(c, s.userTag.Id())
	apiInfo := s.APIInfo(c)
	baseURL, err := url.Parse(fmt.Sprintf("https://%s/", apiInfo.Addrs[0]))
	c.Assert(err, jc.ErrorIsNil)
	s.baseURL = baseURL
	s.modelUUID = s.ControllerModelUUID()
}

func (s *toolsWithMacaroonsIntegrationSuite) TestWithNoBasicAuthReturnsDischargeRequiredError(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
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

func (s *toolsWithMacaroonsIntegrationSuite) TestCanPostWithDischargedMacaroon(c *gc.C) {
	checkCount := 0
	s.DischargerLogin = func() string {
		checkCount++
		return s.userTag.Id()
	}
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Do:     s.doer(),
		Method: "POST",
		URL:    s.toolsURI(""),
	})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
	c.Assert(checkCount, gc.Equals, 1)
}

func (s *toolsWithMacaroonsIntegrationSuite) TestCanPostWithLocalLogin(c *gc.C) {
	// Create a new local user that we can log in as
	// using macaroon authentication.
	password := "hunter2"
	userService := s.ControllerServiceFactory(c).User()
	userTag := names.NewUserTag("bobbrown")
	_, _, err := userService.AddUser(context.Background(), service.AddUserArg{
		Name:        userTag.Name(),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(password)),
	})
	c.Assert(err, jc.ErrorIsNil)

	// TODO (stickupkid): Permissions: This is only required to insert admin
	// permissions into the state, remove when permissions are written to state.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeUser(c, &factory.UserParams{
		Name: userTag.Name(),
	})

	// Install a "web-page" visitor that deals with the interaction
	// method that Juju controllers support for authenticating local
	// users. Note: the use of httpbakery.NewMultiVisitor is necessary
	// to trigger httpbakery to query the authentication methods and
	// bypass browser authentication.
	var prompted bool
	bakeryClient := httpbakery.NewClient()
	jar := jujutesting.NewClearableCookieJar()
	client := jujuhttp.NewClient(
		jujuhttp.WithSkipHostnameVerification(true),
		jujuhttp.WithCookieJar(jar),
	)
	bakeryClient.Client = client.Client()
	bakeryClient.AddInteractor(apiauthentication.NewInteractor(
		userTag.Id(),
		func(username string) (string, error) {
			c.Assert(username, gc.Equals, userTag.Id())
			prompted = true
			return password, nil
		},
	))
	bakeryDo := func(req *http.Request) (*http.Response, error) {
		c.Logf("req.URL: %#v", req.URL)
		return bakeryClient.DoWithCustomError(req, bakeryGetError)
	}

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "POST",
		URL:      s.toolsURI(""),
		Tag:      userTag.String(),
		Password: "", // no password forces macaroon usage
		Do:       bakeryDo,
	})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
	c.Assert(prompted, jc.IsTrue)
}

// doer returns a Do function that can make a bakery request
// appropriate for a charms endpoint.
func (s *toolsWithMacaroonsIntegrationSuite) doer() func(*http.Request) (*http.Response, error) {
	return bakeryDo(nil, bakeryGetError)
}

// bakeryDo provides a function suitable for using in HTTPRequestParams.Do
// that will use the given http client (or bakery created client with a
// non verifying secure TLS config if client is nil) and use the given
// getBakeryError function to translate errors in responses.
func bakeryDo(client *http.Client, getBakeryError func(*http.Response) error) func(*http.Request) (*http.Response, error) {
	bclient := httpbakery.NewClient()
	if client != nil {
		bclient.Client = client
	} else {
		// Configure the default client to skip verification/
		tlsConfig := jujuhttp.SecureTLSConfig()
		tlsConfig.InsecureSkipVerify = true
		bclient.Client.Transport = jujuhttp.NewHTTPTLSTransport(jujuhttp.TransportConfig{
			TLSConfig: tlsConfig,
		})
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
	data, err := io.ReadAll(resp.Body)
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
