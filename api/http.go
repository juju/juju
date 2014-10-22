// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"path"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/api/base"
)

var _ base.HTTPRequestBuilder = (*State)(nil)
var _ base.HTTPCaller = (*State)(nil)

// HTTPClient sends an HTTP request, returning the subsequent response.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

var newHTTPClient = func(state *State) HTTPClient {
	return state.NewHTTPClient()
}

// GetHTTPClient returns an HTTP client initialized based on State.
func (s *State) NewHTTPClient() HTTPClient {
	// For reference, call utils.GetNonValidatingHTTPClient() to get a
	// non-validating client.
	httpclient := utils.GetValidatingHTTPClient()
	tlsconfig := tls.Config{RootCAs: s.certPool, ServerName: "anything"}
	httpclient.Transport = utils.NewHttpTLSTransport(&tlsconfig)
	return httpclient
}

// NewHTTPRequest returns a new API-supporting HTTP request based on State.
func (s *State) NewHTTPRequest(method, path string) (*base.HTTPRequest, error) {
	baseURL, err := url.Parse(s.serverRoot)
	if err != nil {
		return nil, errors.Annotatef(err, "while parsing base URL (%s)", s.serverRoot)
	}

	tag, err := s.EnvironTag()
	if err != nil {
		return nil, errors.Annotate(err, "while extracting environment UUID")
	}
	uuid := tag.Id()

	req, err := newHTTPRequest(method, baseURL, path, uuid, s.tag, s.password)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &base.HTTPRequest{*req}, nil
}

func newHTTPRequest(method string, URL *url.URL, pth, uuid, tag, pw string) (*http.Request, error) {
	URL.Path = path.Join("/environment", uuid, pth)
	req, err := http.NewRequest(method, URL.String(), nil)
	if err != nil {
		return nil, errors.Annotate(err, "while building HTTP request")
	}
	req.SetBasicAuth(tag, pw)
	return req, nil
}

// SendHTTPRequest sends the request using the HTTP client derived from
// State.
func (s *State) SendHTTPRequest(req *base.HTTPRequest) (*http.Response, error) {
	httpclient := newHTTPClient(s)
	resp, err := httpclient.Do(&req.Request)
	if err != nil {
		return nil, errors.Annotate(err, "error when sending HTTP request")
	}
	return resp, nil
}
