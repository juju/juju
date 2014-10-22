// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"net/http"
	"net/url"
	"path"

	"github.com/juju/errors"
)

// NewRequest returns a new HTTP request suitable for the API.
func NewRequest(method string, baseURL *url.URL, pth, uuid, tag, pw string) (*Request, error) {
	baseURL.Path = path.Join("/environment", uuid, pth)
	req, err := http.NewRequest(method, baseURL.String(), nil)
	if err != nil {
		return nil, errors.Annotate(err, "while building HTTP request")
	}
	req.SetBasicAuth(tag, pw)
	return &Request{*req}, nil
}
