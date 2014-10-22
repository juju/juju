// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"crypto/tls"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/utils"

	apihttp "github.com/juju/juju/api/http"
)

var _ apihttp.Client = (*State)(nil)

var newHTTPClient = func(s *State) apihttp.HTTPClient {
	return s.NewHTTPClient()
}

// NewHTTPClient returns an HTTP client initialized based on State.
func (s *State) NewHTTPClient() *http.Client {
	// for reference:
	// Call utils.GetNonValidatingHTTPClient() to get a non-validating client.
	httpclient := utils.GetValidatingHTTPClient()
	tlsconfig := tls.Config{RootCAs: s.certPool, ServerName: "anything"}
	httpclient.Transport = utils.NewHttpTLSTransport(&tlsconfig)
	return httpclient
}

// NewHTTPRequest returns a new API-supporting HTTP request based on State.
func (s *State) NewHTTPRequest(method, path string) (*apihttp.Request, error) {
	baseURL, err := url.Parse(s.serverRoot)
	if err != nil {
		return nil, errors.Annotatef(err, "while parsing base URL (%s)", s.serverRoot)
	}

	tag, err := s.EnvironTag()
	if err != nil {
		return nil, errors.Annotate(err, "while extracting environment UUID")
	}
	uuid := tag.Id()

	req, err := apihttp.NewRequest(method, baseURL, path, uuid, s.tag, s.password)
	return req, errors.Trace(err)
}

// SendHTTPRequest sends the request using the HTTP client derived from State.
func (s *State) SendHTTPRequest(req *apihttp.Request) (*http.Response, error) {
	httpclient := newHTTPClient(s)
	resp, err := httpclient.Do(&req.Request)
	if err != nil {
		return nil, errors.Annotate(err, "while sending HTTP request")
	}
	return resp, nil
}
