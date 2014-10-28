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
	apiserverhttp "github.com/juju/juju/apiserver/http"
)

var newHTTPClient = func(s *State) apihttp.HTTPClient {
	return s.NewHTTPClient()
}

// NewHTTPClient returns an HTTP client initialized based on State.
func (s *State) NewHTTPClient() *http.Client {
	httpclient := utils.GetValidatingHTTPClient()
	tlsconfig := tls.Config{
		RootCAs: s.certPool,
		// We want to be specific here (rather than just using "anything".
		// See commit 7fc118f015d8480dfad7831788e4b8c0432205e8 (PR 899).
		ServerName: "juju-apiserver",
	}
	httpclient.Transport = utils.NewHttpTLSTransport(&tlsconfig)
	return httpclient
}

// NewHTTPRequest returns a new API-supporting HTTP request based on State.
func (s *State) NewHTTPRequest(method, path string) (*http.Request, error) {
	baseURL, err := url.Parse(s.serverRoot)
	if err != nil {
		return nil, errors.Annotatef(err, "while parsing base URL (%s)", s.serverRoot)
	}

	tag, err := s.EnvironTag()
	if err != nil {
		return nil, errors.Annotate(err, "while extracting environment UUID")
	}
	uuid := tag.Id()

	req, err := apiserverhttp.NewRequest(method, baseURL, path, uuid, s.tag, s.password)
	return req, errors.Trace(err)
}

// SendHTTPRequest sends a request using the HTTP client derived from State.
func (s *State) SendHTTPRequest(method, path string, args interface{}) (*http.Request, *http.Response, error) {
	req, err := s.NewHTTPRequest(method, path)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	err = apiserverhttp.SetRequestArgs(req, args)
	if err != nil {
		return nil, nil, errors.Annotate(err, "while setting request body")
	}

	httpclient := newHTTPClient(s)
	resp, err := httpclient.Do(req)
	if err != nil {
		return nil, nil, errors.Annotate(err, "while sending HTTP request")
	}
	return req, resp, nil
}
