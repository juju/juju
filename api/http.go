// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

var sendHTTPRequest = func(r *http.Request, c *http.Client) (*http.Response, error) {
	return c.Do(r)
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

func newHTTPRequest(method string, URL *url.URL, path, uuid, tag, pw string) (*http.Request, error) {
	URL.Path = fmt.Sprintf("/environment/%s/%s", uuid, path)
	req, err := http.NewRequest(method, URL.String(), nil)
	if err != nil {
		return nil, errors.Annotate(err, "while building HTTP request")
	}
	req.SetBasicAuth(tag, pw)
	return req, nil
}

func (s *State) getHTTPClient(secure bool) *http.Client {
	var httpclient *http.Client
	if secure {
		httpclient = utils.GetValidatingHTTPClient()
		tlsconfig := tls.Config{RootCAs: s.certPool, ServerName: "anything"}
		httpclient.Transport = utils.NewHttpTLSTransport(&tlsconfig)
	} else {
		httpclient = utils.GetNonValidatingHTTPClient()
	}
	return httpclient
}

// SendHTTPRequest sends the request using the HTTP client derived from
// State.
func (s *State) SendHTTPRequest(req *http.Request) (*http.Response, error) {
	secure := true
	httpclient := s.getHTTPClient(secure)
	resp, err := sendHTTPRequest(req, httpclient)
	if err != nil {
		return nil, errors.Annotate(err, "error when sending HTTP request")
	}
	return resp, nil
}
