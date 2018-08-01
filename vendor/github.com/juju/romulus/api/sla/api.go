// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sla contains the sla service API client.
package sla

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"

	"github.com/juju/romulus"
	"github.com/juju/romulus/wireformat/common"
	"github.com/juju/romulus/wireformat/sla"
)

type httpErrorResponse struct {
	Error string `json:"error"`
}

// AuthClient defines the interface available to clients of the support api.
type AuthClient interface {
	// Authorize returns the sla macaroon for the specified model
	Authorize(modelUUID, supportLevel, budget string) (*sla.SLAResponse, error)
}

var _ AuthClient = (*client)(nil)

type httpClient interface {
	DoWithBody(req *http.Request, body io.ReadSeeker) (*http.Response, error)
}

// client is the implementation of the Client interface.
type client struct {
	client  httpClient
	apiRoot string
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

// APIRoot sets the base url for the api client.
func APIRoot(apiRoot string) func(h *client) error {
	return func(h *client) error {
		h.apiRoot = apiRoot
		return nil
	}
}

// NewClient returns a new client for the sla api.
func NewClient(options ...ClientOption) (*client, error) {
	c := &client{
		client:  httpbakery.NewClient(),
		apiRoot: romulus.DefaultAPIRoot,
	}

	for _, option := range options {
		err := option(c)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return c, nil
}

// Authorize obtains an sla authorization.
func (c *client) Authorize(modelUUID, supportLevel, budget string) (*sla.SLAResponse, error) {
	u, err := url.Parse(c.apiRoot + "/sla/authorize")
	if err != nil {
		return nil, errors.Trace(err)
	}

	slaRequest := sla.SLARequest{
		ModelUUID: modelUUID,
		Level:     supportLevel,
		Budget:    budget,
	}

	buff := &bytes.Buffer{}
	encoder := json.NewEncoder(buff)
	err = encoder.Encode(slaRequest)
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
		respErr := httpErrorResponse{}
		json.NewDecoder(response.Body).Decode(&respErr)
		return nil, common.HTTPError{
			StatusCode: response.StatusCode,
			Message:    respErr.Error,
		}
	}

	var respDoc sla.SLAResponse
	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(&respDoc)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to unmarshal the response")
	}

	return &respDoc, nil
}

// discardClose reads any remaining data from the response body and closes it.
func discardClose(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	io.Copy(ioutil.Discard, response.Body)
	response.Body.Close()
}
