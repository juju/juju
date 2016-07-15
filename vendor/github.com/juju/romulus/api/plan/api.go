// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package plan contains the plan service API client.
package plan

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	wireformat "github.com/juju/romulus/wireformat/plan"
)

var DefaultURL = "https://api.jujucharms.com/omnibus/v2"

// Client defines the interface available to clients of the plan api.
type Client interface {
	// GetAssociatedPlans returns the plans associated with the charm.
	GetAssociatedPlans(charmURL string) ([]wireformat.Plan, error)
}

// AuthorizationClient defines the interface available to clients of the public plan api.
type AuthorizationClient interface {
	// Authorize returns the authorization macaroon for the specified environment, charm url and service name.
	Authorize(environmentUUID, charmURL, serviceName, plan string, visitWebPage func(*url.URL) error) (*macaroon.Macaroon, error)
}

var _ Client = (*client)(nil)
var _ AuthorizationClient = (*client)(nil)

type httpClient interface {
	// Do sends the given HTTP request and returns its response.
	Do(*http.Request) (*http.Response, error)
	// DoWithBody is like Do except that the given body is used
	// for the body of the HTTP request, and reset to its start
	// by seeking if the request is retried. It is an error if
	// req.Body is non-zero.
	DoWithBody(req *http.Request, body io.ReadSeeker) (*http.Response, error)
}

// client is the implementation of the Client interface.
type client struct {
	client  httpClient
	baseURL string
}

// ClientOption defines a function which configures a Client.
type ClientOption func(h *client) error

// HTTPClient returns a function that sets the http client used by the API
// (e.g. if we want to use TLS).
func HTTPClient(c httpClient) func(h *client) error {
	return func(h *client) error {
		h.client = c
		return nil
	}
}

// BaseURL sets the base url for the api client.
func BaseURL(url string) func(h *client) error {
	return func(h *client) error {
		h.baseURL = url
		return nil
	}
}

// NewAuthorizationClient returns a new public authorization client.
func NewAuthorizationClient(options ...ClientOption) (AuthorizationClient, error) {
	return NewClient(options...)
}

// NewClient returns a new client for plan management.
func NewClient(options ...ClientOption) (*client, error) {
	c := &client{
		client:  httpbakery.NewClient(),
		baseURL: DefaultURL,
	}

	for _, option := range options {
		err := option(c)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return c, nil
}

// GetAssociatedPlans returns the default plan for the specified charm.
func (c *client) GetAssociatedPlans(charmURL string) ([]wireformat.Plan, error) {
	u, err := url.Parse(c.baseURL + "/charm")
	if err != nil {
		return nil, errors.Trace(err)
	}
	query := u.Query()
	query.Set("charm-url", charmURL)
	u.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create GET request")
	}
	response, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Annotate(err, "failed to retrieve associated plans")
	}
	defer discardClose(response)

	if response.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(response.Body)
		if err == nil {
			return nil, errors.Errorf("failed to retrieve associated plans: received http response: %v - code %q", string(body), http.StatusText(response.StatusCode))
		}
		return nil, errors.Errorf("failed to retrieve associated plans: received http response: %q", http.StatusText(response.StatusCode))
	}
	var plans []wireformat.Plan
	dec := json.NewDecoder(response.Body)
	err = dec.Decode(&plans)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to unmarshal response")
	}
	return plans, nil
}

// Authorize implements the AuthorizationClient.Authorize method.
func (c *client) Authorize(environmentUUID, charmURL, serviceName, planURL string, visitWebPage func(*url.URL) error) (*macaroon.Macaroon, error) {
	u, err := url.Parse(c.baseURL + "/plan/authorize")
	if err != nil {
		return nil, errors.Trace(err)
	}

	auth := wireformat.AuthorizationRequest{
		EnvironmentUUID: environmentUUID,
		CharmURL:        charmURL,
		ServiceName:     serviceName,
		PlanURL:         planURL,
	}

	buff := &bytes.Buffer{}
	encoder := json.NewEncoder(buff)
	err = encoder.Encode(auth)
	if err != nil {
		return nil, errors.Trace(err)
	}

	req, err := http.NewRequest("POST", u.String(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := c.client.DoWithBody(req, bytes.NewReader(buff.Bytes()))
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer discardClose(response)

	if response.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(response.Body)
		if err == nil {
			return nil, errors.Errorf("failed to authorize plan: received http response: %v - code %q", string(body), http.StatusText(response.StatusCode))
		}
		return nil, errors.Errorf("failed to authorize plan: http response is %q", http.StatusText(response.StatusCode))
	}

	var m *macaroon.Macaroon
	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(&m)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to unmarshal the response")
	}

	return m, nil
}

// discardClose reads any remaining data from the response body and closes it.
func discardClose(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	io.Copy(ioutil.Discard, response.Body)
	response.Body.Close()
}
