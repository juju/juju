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
)

// Doer sends an HTTP request, returning the subsequent response.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

var newHTTPClient = func(state *State) Doer {
	return state.GetHTTPClient()
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

	req, err := newHTTPRequest(method, baseURL, path, uuid, s.tag, s.password)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return req, nil
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

func (s *State) GetHTTPClient() Doer {
	// For reference, call utils.GetNonValidatingHTTPClient() to get a
	// non-validating client.
	httpclient := utils.GetValidatingHTTPClient()
	tlsconfig := tls.Config{RootCAs: s.certPool, ServerName: "anything"}
	httpclient.Transport = utils.NewHttpTLSTransport(&tlsconfig)
	return httpclient
}

// SendHTTPRequest sends the request using the HTTP client derived from
// State.
func (s *State) SendHTTPRequest(req *http.Request) (*http.Response, error) {
	httpclient := newHTTPClient(s)
	resp, err := httpclient.Do(req)
	if err != nil {
		return nil, errors.Annotate(err, "error when sending HTTP request")
	}
	return resp, nil
}
