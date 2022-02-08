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
	}
	return nil
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
func (t basicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.authorizeRequest(req); err != nil {
		return nil, errors.Trace(err)
	}
	resp, err := t.transport.RoundTrip(req)
	logger.Tracef("basicTransport %q, resp.Header => %#v, %q", req.URL, resp.Header, resp.Status)
	return resp, errors.Trace(err)
}

type tokenTransport struct {
	transport       http.RoundTripper
	username        string
	password        string
	authToken       string
	OAuthToken      string
	reuseOAuthToken bool
}

func newTokenTransport(
	transport http.RoundTripper, username, password, authToken, OAuthToken string, reuseOAuthToken bool,
) http.RoundTripper {
	return &tokenTransport{
		transport:       transport,
		username:        username,
		password:        password,
		authToken:       authToken,
		OAuthToken:      OAuthToken,
		reuseOAuthToken: reuseOAuthToken,
	}
}

func (tokenTransport) scheme() string {
	return "Bearer"
}

func getChallengeParameters(scheme string, resp *http.Response) map[string]string {
	logger.Tracef(
		"getting chanllenge parametter for %q with scheme %q from %q",
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
	if len(parameters) == 0 {
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
	if scope != "" {
		q.Set("scope", scope)
	}
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
	if t.OAuthToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("%s %s", t.scheme(), t.OAuthToken))
	}
	return nil
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	defer func() {
		if !t.reuseOAuthToken {
			// We usually do not re-use the OAuth token because each API call might have different scope.
			// But some of the provider use long life token and there is no need to refresh.
			t.OAuthToken = ""
		}
	}()

	if err := t.authorizeRequest(req); err != nil {
		return nil, errors.Trace(err)
	}
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if isUnauthorizedResponse(resp) {
		// refresh token and retry.
		return t.retry(req, resp)
	}
	return resp, errors.Trace(err)
}

func (t *tokenTransport) retry(req *http.Request, prevResp *http.Response) (*http.Response, error) {
	logger.Tracef(
		"retrying req URL %q, previous response header %#v, status %v",
		req.URL, prevResp.Header, prevResp.Status,
	)

	if err := t.refreshOAuthToken(prevResp); err != nil {
		return nil, errors.Annotatef(err, "refreshing OAuth token")
	}
	if err := t.authorizeRequest(req); err != nil {
		return nil, errors.Trace(err)
	}
	resp, err := t.transport.RoundTrip(req)
	if isUnauthorizedResponse(resp) {
		if t.password == "" && t.authToken == "" {
			return nil, errors.NewUnauthorized(err, "authorization is required for a private registry")
		}
	}
	return resp, errors.Trace(err)
}

func isUnauthorizedResponse(resp *http.Response) bool {
	return resp != nil && resp.StatusCode == http.StatusUnauthorized
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
	if err != nil {
		return resp, errors.Trace(err)
	}
	if resp.StatusCode < 400 {
		return resp, nil
	}
	logger.Tracef("errorTransport %q, err -> %v", request.URL, err)
	return handleErrorResponse(resp)
}

func handleErrorResponse(resp *http.Response) (*http.Response, error) {
	if resp.StatusCode < 400 {
		return resp, nil
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Annotatef(err, "reading bad response body with status code %d", resp.StatusCode)
	}
	errMsg := fmt.Sprintf("non-successful response status=%d", resp.StatusCode)
	if logger.IsTraceEnabled() {
		logger.Tracef("%s, url %q, body=%q", errMsg, resp.Request.URL.String(), body)
	}
	errNew := errors.Errorf
	switch resp.StatusCode {
	case http.StatusForbidden:
		errNew = errors.Forbiddenf
	case http.StatusUnauthorized:
		errNew = errors.Unauthorizedf
	}
	return nil, errNew(errMsg)
}
