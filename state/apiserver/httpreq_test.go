// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/testing/factory"
)

// TODO (ericsnow) BUG#1336542 Update the following to use the common code:
// charms_test.go
// tools_test.go
// debuglog_test.go

type authHttpSuite struct {
	jujutesting.JujuConnSuite
	userTag            string
	password           string
	archiveContentType string
	apiBinding         string
	apiArgs            string // We could also use *url.Values.
	httpMethod         string
	httpClient         httpDoer
}

func (s *authHttpSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.password = "password"
	user := s.Factory.MakeUser(factory.UserParams{Password: s.password})
	s.userTag = user.Tag().String()
	// Set default.
	s.httpMethod = "POST"
	s.httpClient = s.APIState.SecureHTTPClient("anything")
}

//---------------------------
// legacy helpers

func (s *authHttpSuite) sendRequest(c *gc.C, tag, password, method, uri, contentType string, body io.Reader) (*http.Response, error) {
	return s.sendRequestRaw(tag, password, method, uri, contentType, body)
}

func (s *authHttpSuite) authRequest(c *gc.C, method, uri, contentType string, body io.Reader) (*http.Response, error) {
	return s.sendRequest(c, s.userTag, s.password, method, uri, contentType, body)
}

func (s *authHttpSuite) uploadRequest(c *gc.C, uri string, asZip bool, path string) (*http.Response, error) {
	URL, err := url.Parse(uri)
	c.Assert(err, gc.IsNil)
	mimetype := ""
	if asZip {
		mimetype = s.archiveContentType
	}
	return s.sendFile(URL, path, mimetype)
}

func (s *authHttpSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string, result params.FailableResult) {
	body := assertResponse(c, resp, expCode, "application/json")
	jsonResponse(c, body, result)
	c.Check(result.Err(), gc.ErrorMatches, expError)
}

func assertResponse(c *gc.C, resp *http.Response, expCode int, expContentType string) []byte {
	c.Check(resp.StatusCode, gc.Equals, expCode)
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	c.Assert(err, gc.IsNil)
	ctype := resp.Header.Get("Content-Type")
	c.Assert(ctype, gc.Equals, expContentType)
	return body
}

func jsonResponse(c *gc.C, body []byte, result params.FailableResult) {
	err := json.Unmarshal(body, &result)
	c.Assert(err, gc.IsNil)
}

//---------------------------
// URL helpers

func (s *authHttpSuite) UUID() (string, error) {
	env, err := s.State.Environment()
	if err != nil {
		return "", err
	}
	return env.UUID(), nil
}

func (s *authHttpSuite) baseURL() (*url.URL, error) {
	_, info, err := s.APIConn.Environ.StateInfo()
	if err != nil {
		return nil, err
	}
	baseurl := &url.URL{
		Scheme: "https",
		Host:   info.Addrs[0],
		Path:   "",
	}
	return baseurl, nil
}

func (s *authHttpSuite) URL(query string) (*url.URL, error) {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	uuid, err := s.UUID()
	if err != nil {
		return nil, err
	}
	// Build the URL.
	uri, err := s.baseURL()
	if err != nil {
		return nil, err
	}
	uri.Path += fmt.Sprintf("/environment/%s/%s", uuid, s.apiBinding)
	uri.RawQuery = query
	return uri, nil
}

func (s *authHttpSuite) legacyURL(query string) (*url.URL, error) {
	URL, err := s.URL(query)
	if err != nil {
		return nil, err
	}
	URL.Path = "/" + s.apiBinding
	return URL, nil
}

//---------------------------
// request

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// XXX Add and httpResource struct (HTTPMethod, URL)?

type httpBasicAuth struct {
	UserID   string
	Password string
}

func (a *httpBasicAuth) AddHeader(req *http.Request) {
	if a.UserID == "" && a.Password == "" {
		return
	}
	req.SetBasicAuth(a.UserID, a.Password)
}

type httpPayload struct {
	Data     io.Reader
	Mimetype string
}

func (p *httpPayload) AddHeader(req *http.Request) {
	mimetype := p.Mimetype
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	req.Header.Set("Content-Type", mimetype)
}

type httpRequest struct {
	Method  string
	URL     *url.URL
	Auth    *httpBasicAuth
	Payload *httpPayload
}

func (s *authHttpSuite) NewRequest(
	method string, URL *url.URL, auth *httpBasicAuth, payload *httpPayload,
) (
	req *httpRequest, err error,
) {
	if method == "" {
		if s.httpMethod == "" {
			return nil, fmt.Errorf("missing HTTP method")
		}
		method = s.httpMethod
	}
	if URL == nil {
		URL, err = s.URL("")
		if err != nil {
			return nil, err
		}
	}
	if auth == nil {
		auth = &httpBasicAuth{s.userTag, s.password}
	}
	return &httpRequest{method, URL, auth, payload}, nil
}

func (r *httpRequest) Raw() (*http.Request, error) {
	if r.URL == nil {
		return nil, fmt.Errorf("missing URL")
	}
	var body io.Reader
	if r.Payload != nil {
		body = r.Payload.Data
	}

	req, err := http.NewRequest(r.Method, r.URL.String(), body)
	if err != nil {
		return nil, err
	}
	if r.Auth != nil {
		r.Auth.AddHeader(req)
	}
	if r.Payload != nil {
		r.Payload.AddHeader(req)
	}
	return req, nil
}

func (r *httpRequest) Send(client httpDoer) (*http.Response, error) {
	req, err := r.Raw()
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

//---------------------------
// request helpers

func (s *authHttpSuite) sendRequestRaw(tag, password, method, uri, contentType string, body io.Reader) (*http.Response, error) {
	URL, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	auth := &httpBasicAuth{tag, password}
	payload := &httpPayload{body, contentType}
	req, err := s.NewRequest(method, URL, auth, payload)
	if err != nil {
		return nil, err
	}
	return req.Send(s.httpClient)
}

func (s *authHttpSuite) sendMethod(URL *url.URL, method string) (*http.Response, error) {
	auth := &httpBasicAuth{s.userTag, s.password}
	req, err := s.NewRequest(method, URL, auth, nil)
	if err != nil {
		return nil, err
	}
	return req.Send(s.httpClient)
}

func (s *authHttpSuite) sendAuth(URL *url.URL, tag, pw string) (*http.Response, error) {
	auth := &httpBasicAuth{tag, pw}
	req, err := s.NewRequest(s.httpMethod, URL, auth, nil)
	if err != nil {
		return nil, err
	}
	return req.Send(s.httpClient)
}

func (s *authHttpSuite) sendURL(URL *url.URL) (*http.Response, error) {
	return s.sendAuth(URL, s.userTag, s.password)
}

func (s *authHttpSuite) sendFile(URL *url.URL, path, mimetype string) (*http.Response, error) {
	// If necessary, we could guess the mimetype from the path.
	var file *os.File
	var err error
	if path != "" {
		file, err = os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
	}

	auth := &httpBasicAuth{s.userTag, s.password}
	payload := &httpPayload{file, mimetype}
	req, err := s.NewRequest(s.httpMethod, URL, auth, payload)
	if err != nil {
		return nil, err
	}
	return req.Send(s.httpClient)
}

//---------------------------
// response helpers

func (s *authHttpSuite) readResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

func (s *authHttpSuite) readJSONResponse(resp *http.Response, result interface{}) (error, error) {
	body, err := s.readResponse(resp)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, result)
	if err != nil {
		return nil, err
	}

	errorResult, ok := result.(params.FailableResult)
	if !ok {
		return nil, nil
	}
	return errorResult.Err(), nil
}

//---------------------------
// response assertions

func (s *authHttpSuite) checkResponse(c *gc.C, resp *http.Response, statusCode int, mimetype string) {
	c.Check(resp.StatusCode, gc.Equals, statusCode)
	ctype := resp.Header.Get("Content-Type")
	c.Check(ctype, gc.Equals, mimetype)
}

func (s *authHttpSuite) checkJSONResponse(c *gc.C, resp *http.Response, result interface{}) {
	s.checkResponse(c, resp, http.StatusOK, "application/json")
	failure, err := s.readJSONResponse(resp, result)
	c.Assert(err, gc.IsNil)
	c.Check(failure, gc.IsNil)
}

func (s *authHttpSuite) checkErrorResponse(c *gc.C, resp *http.Response, statusCode int, msg string, result params.FailableResult) {
	s.checkResponse(c, resp, statusCode, "application/json")
	failure, err := s.readJSONResponse(resp, result)
	c.Assert(err, gc.IsNil)
	c.Check(failure, gc.ErrorMatches, msg)
}

func (s *authHttpSuite) checkFileResponse(c *gc.C, resp *http.Response, expected, mimetype string) {
	s.checkResponse(c, resp, http.StatusOK, mimetype)
	body, err := s.readResponse(resp)
	c.Assert(err, gc.IsNil)
	c.Check(string(body), gc.Equals, expected)
}

//---------------------------
// HTTP assertions

func (s *authHttpSuite) checkServedSecurely(c *gc.C) {
	URL, err := s.URL("")
	c.Assert(err, gc.IsNil)
	URL.Scheme = "http"

	_, err = s.sendURL(URL)
	c.Check(err, gc.ErrorMatches, `.*malformed HTTP response.*`)
}

func (s *authHttpSuite) checkHTTPMethodInvalid(c *gc.C, method string, result params.FailableResult) {
	URL, err := s.URL("")
	c.Assert(err, gc.IsNil)

	resp, err := s.sendMethod(URL, method)
	c.Assert(err, gc.IsNil)
	s.checkErrorResponse(c, resp, http.StatusMethodNotAllowed, "unsupported method: .*", result)
}

func (s *authHttpSuite) checkLegacyPath(c *gc.C, query, expected string) {
	url, err := s.baseURL()
	c.Assert(err, gc.IsNil)
	url.RawQuery = query
	url.Path = "/" + s.apiBinding

	resp, err := s.sendURL(url)
	c.Assert(err, gc.IsNil)
	if expected != "" {
		s.checkFileResponse(c, resp, expected, "text/plain; charset=utf-8")
	} else {
		c.Check(resp.StatusCode, gc.Equals, http.StatusNotFound)
	}
}

func (s *authHttpSuite) checkLegacyPathAvailable(c *gc.C, query, expected string) {
	s.checkLegacyPath(c, query, expected)
}

func (s *authHttpSuite) checkLegacyPathUnavailable(c *gc.C, query string) {
	s.checkLegacyPath(c, query, "")
}

//---------------------------
// auth assertions

func (s *authHttpSuite) checkRequiresAuth(c *gc.C, result params.FailableResult) {
	URL, err := s.URL("")
	c.Assert(err, gc.IsNil)
	tag := ""
	pw := ""
	resp, err := s.sendAuth(URL, tag, pw)
	c.Assert(err, gc.IsNil)
	s.checkErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized", result)
}

func (s *authHttpSuite) checkRequiresUser(c *gc.C, result params.FailableResult) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Check(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Check(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Check(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Check(err, gc.IsNil)

	URL, err := s.URL("")
	c.Assert(err, gc.IsNil)

	// Try to log in unsuccessfully.
	resp, err := s.sendAuth(URL, machine.Tag().String(), password)
	c.Assert(err, gc.IsNil)
	s.checkErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized", result)

	// Try to log in successfully.
	// (With an invalid method so we don't actually do anything.)
	s.checkHTTPMethodInvalid(c, "PUT", result)
}

//---------------------------
// environment assertions

func (s *authHttpSuite) checkBindingAllowsEnvUUIDPath(c *gc.C, query, expected string) {
	url, err := s.URL(query)
	c.Assert(err, gc.IsNil)
	resp, err := s.sendURL(url)
	c.Assert(err, gc.IsNil)
	s.checkFileResponse(c, resp, expected, "text/plain; charset=utf-8")
}

func (s *authHttpSuite) checkBindingWrongEnv(c *gc.C, query string, result params.FailableResult) {
	url, err := s.URL(query)
	c.Assert(err, gc.IsNil)
	url.Path = "/environment/dead-beef-123456/" + s.apiBinding
	resp, err := s.sendURL(url)
	c.Assert(err, gc.IsNil)
	s.checkErrorResponse(c, resp, http.StatusNotFound, `unknown environment: "dead-beef-123456"`, result)
}
