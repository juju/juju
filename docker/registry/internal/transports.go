// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/docker/distribution/registry/client/auth/challenge"
	"github.com/juju/errors"
)

type basicTransport struct {
	transport http.RoundTripper
	username  string
	password  string
	authToken string
}

func newBasicTransport(
	transport http.RoundTripper, username string, password string, authToken string,
) http.RoundTripper {
	return &basicTransport{
		transport: transport,
		username:  username,
		password:  password,
		authToken: authToken,
	}
}

func (basicTransport) scheme() string {
	return "Basic"
}

func (t basicTransport) authorizeRequest(req *http.Request) error {
	if t.authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("%s %s", t.scheme(), t.authToken))
		return nil
	}
	if t.username != "" || t.password != "" {
		req.SetBasicAuth(t.username, t.password)
		return nil
	}
	return errors.NotValidf("no basic auth credentials")
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
func (t basicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.authorizeRequest(req); err != nil {
		return nil, errors.Trace(err)
	}
	resp, err := t.transport.RoundTrip(req)
	logger.Tracef("basicTransport req.Header => %#v, resp.Header => %#v", req.Header, resp.Header)
	return resp, errors.Trace(err)
}

type tokenTransport struct {
	transport  http.RoundTripper
	username   string
	password   string
	authToken  string
	OAuthToken string
}

func newTokenTransport(
	transport http.RoundTripper, username, password, authToken, OAuthToken string,
) http.RoundTripper {
	return &tokenTransport{
		transport:  transport,
		username:   username,
		password:   password,
		authToken:  authToken,
		OAuthToken: OAuthToken,
	}
}

func (tokenTransport) scheme() string {
	return "Bearer"
}

func getChallengeParameters(scheme string, resp *http.Response) map[string]string {
	logger.Tracef(
		"getting chanllenge parametter for with %q (%q) from %q",
		resp.Request.URL.String(),
		scheme, resp.Header[http.CanonicalHeaderKey("WWW-Authenticate")],
	)
	for _, c := range challenge.ResponseChallenges(resp) {
		if strings.EqualFold(c.Scheme, scheme) {
			return c.Parameters
		}
	}
	logger.Tracef("failed to get challenge parameters for %q schema -> %v", scheme, resp.Header)
	return nil
}

type tokenResponse struct {
	Token        string    `json:"token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int       `json:"expires_in"`
	IssuedAt     time.Time `json:"issued_at"`
	Scope        string    `json:"scope"`
}

func (t tokenResponse) token() string {
	if t.AccessToken != "" {
		return t.AccessToken
	}
	if t.Token != "" {
		return t.Token
	}
	return ""
}

func (t *tokenTransport) refreshOAuthToken(failedResp *http.Response) error {
	t.OAuthToken = ""

	parameters := getChallengeParameters(t.scheme(), failedResp)
	if parameters == nil {
		return errors.NewForbidden(nil, "failed to refresh bearer token")
	}
	realm, ok := parameters["realm"]
	if !ok {
		return errors.New("no realm specified for token auth challenge")
	}
	service, ok := parameters["service"]
	if !ok {
		return errors.New("no service specified for token auth challenge")
	}
	scope, ok := parameters["scope"]
	if !ok {
		logger.Tracef("no scope specified for token auth challenge")
	}

	url, err := url.Parse(realm)
	if err != nil {
		return errors.Trace(err)
	}
	q := url.Query()
	q.Set("scope", scope)
	q.Set("service", service)
	url.RawQuery = q.Encode()

	request, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return errors.Trace(err)
	}
	tokenRefreshTransport := newBasicTransport(t.transport, t.username, t.password, t.authToken)
	resp, err := tokenRefreshTransport.RoundTrip(request)
	if err != nil {
		return errors.Trace(err)
	}
	if resp.StatusCode != http.StatusOK {
		_, err = handleErrorResponse(resp)
		return errors.Trace(err)
	}

	decoder := json.NewDecoder(resp.Body)
	var tr tokenResponse
	if err = decoder.Decode(&tr); err != nil {
		return fmt.Errorf("unable to decode token response: %s", err)
	}
	t.OAuthToken = tr.token()
	return nil
}

func (t *tokenTransport) authorizeRequest(req *http.Request) error {
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", t.scheme(), t.OAuthToken))
	return nil
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.authorizeRequest(req); err != nil {
		return nil, errors.Trace(err)
	}
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resp != nil && resp.StatusCode == http.StatusUnauthorized {
		// refresh token and retry.
		return t.retry(req, resp)
	}
	return resp, errors.Trace(err)
}

func (t *tokenTransport) retry(req *http.Request, prevResp *http.Response) (*http.Response, error) {
	if err := t.refreshOAuthToken(prevResp); err != nil {
		return nil, errors.Annotatef(err, "refreshing OAuth token")
	}
	if err := t.authorizeRequest(req); err != nil {
		return nil, errors.Trace(err)
	}
	resp, err := t.transport.RoundTrip(req)
	return resp, errors.Trace(err)
}

type errorTransport struct {
	transport http.RoundTripper
}

func newErrorTransport(transport http.RoundTripper) http.RoundTripper {
	return &errorTransport{transport: transport}
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
func (t errorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	resp, err := t.transport.RoundTrip(request)
	logger.Tracef(
		"errorTransport request.URL -> %q, request.Header -> %#v, err -> %v",
		request.URL, request.Header, err,
	)
	if err != nil {
		return resp, err
	}
	return handleErrorResponse(resp)
}

func handleErrorResponse(resp *http.Response) (*http.Response, error) {
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Annotatef(err, "reading bad response body with status code %d", resp.StatusCode)
		}
		errMsg := fmt.Sprintf("non-successful response status=%d", resp.StatusCode)
		logger.Tracef("%s, url %q, body=%q", errMsg, resp.Request.URL.String(), body)
		return nil, errors.New(errMsg)
	}
	return resp, nil
}
