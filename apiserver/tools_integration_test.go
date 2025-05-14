// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiauthentication "github.com/juju/juju/api/authentication"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	jujuhttp "github.com/juju/juju/internal/http"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
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

func (s *toolsCommonSuite) assertJSONErrorResponse(c *tc.C, resp *http.Response, expCode int, expError string) {
	toolsResponse := assertResponse(c, resp, expCode)
	c.Check(toolsResponse.ToolsList, tc.IsNil)
	c.Check(toolsResponse.Error, tc.NotNil)
	c.Check(toolsResponse.Error.Message, tc.Matches, expError)
}

// URL returns a URL for this server with the given path and
// query parameters. The URL scheme will be "https".
func (s *toolsCommonSuite) URL(path string, queryParams url.Values) *url.URL {
	url := *s.baseURL
	url.Path = path
	url.RawQuery = queryParams.Encode()
	return &url
}

func assertResponse(c *tc.C, resp *http.Response, expStatus int) params.ToolsResult {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var toolsResponse params.ToolsResult
	err := json.Unmarshal(body, &toolsResponse)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("body: %s", body))
	return toolsResponse
}

type toolsWithMacaroonsIntegrationSuite struct {
	toolsCommonSuite
	jujutesting.MacaroonSuite
	userName user.Name
}

var _ = tc.Suite(&toolsWithMacaroonsIntegrationSuite{})

func (s *toolsWithMacaroonsIntegrationSuite) SetUpTest(c *tc.C) {
	s.MacaroonSuite.SetUpTest(c)

	s.userName = usertesting.GenNewName(c, "bob@authhttpsuite")
	s.AddModelUser(c, s.userName)
	s.AddControllerUser(c, s.userName, permission.LoginAccess)

	apiInfo := s.APIInfo(c)
	baseURL, err := url.Parse(fmt.Sprintf("https://%s/", apiInfo.Addrs[0]))
	c.Assert(err, tc.ErrorIsNil)

	s.baseURL = baseURL
	s.modelUUID = s.ControllerModelUUID()
}

func (s *toolsWithMacaroonsIntegrationSuite) TestWithNoBasicAuthReturnsDischargeRequiredError(c *tc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method: "POST",
		URL:    s.toolsURI(""),
	})

	charmResponse := assertResponse(c, resp, http.StatusUnauthorized)
	c.Assert(charmResponse.Error, tc.NotNil)
	c.Assert(charmResponse.Error.Message, tc.Equals, "macaroon discharge required: authentication required")
	c.Assert(charmResponse.Error.Code, tc.Equals, params.CodeDischargeRequired)
	c.Assert(charmResponse.Error.Info, tc.NotNil)
	c.Assert(charmResponse.Error.Info["bakery-macaroon"], tc.NotNil)
}

func (s *toolsWithMacaroonsIntegrationSuite) TestCanPostWithDischargedMacaroon(c *tc.C) {
	checkCount := 0
	s.DischargerLogin = func() string {
		checkCount++
		return s.userName.Name()
	}
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Do:     s.doer(),
		Method: "POST",
		URL:    s.toolsURI(""),
	})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
	c.Assert(checkCount, tc.Equals, 1)
}

func (s *toolsWithMacaroonsIntegrationSuite) TestCanPostWithLocalLogin(c *tc.C) {
	// Create a new local user that we can log in as
	// using macaroon authentication.
	password := "hunter2"
	accessService := s.ControllerDomainServices(c).Access()
	userName := usertesting.GenNewName(c, "bobbrown")
	_, _, err := accessService.AddUser(c.Context(), service.AddUserArg{
		Name:        userName,
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(password)),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

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
		userName.Name(),
		func(username string) (string, error) {
			c.Assert(username, tc.Equals, userName.Name())
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
		Tag:      names.NewUserTag(userName.Name()).String(),
		Password: "", // no password forces macaroon usage
		Do:       bakeryDo,
	})
	s.assertJSONErrorResponse(c, resp, http.StatusBadRequest, "expected binaryVersion argument")
	c.Assert(prompted, tc.IsTrue)
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
		// Configure the default client to skip verification.
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
		return errors.Annotatef(err, "cannot unmarshal body %q", string(data))
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
