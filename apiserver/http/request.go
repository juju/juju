// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	"github.com/juju/errors"
)

// NewRequest returns a new HTTP request suitable for the API.
func NewRequest(method string, baseURL *url.URL, pth, uuid, tag, pw string) (*http.Request, error) {
	baseURL.Path = path.Join("/environment", uuid, pth)

	req, err := http.NewRequest(method, baseURL.String(), nil)
	if err != nil {
		return nil, errors.Annotate(err, "while building HTTP request")
	}

	req.SetBasicAuth(tag, pw)
	return req, nil
}

// SetRequestArgs JSON-encodes the args and sets them as the request body.
func SetRequestArgs(req *http.Request, args interface{}) error {
	data, err := json.Marshal(args)
	if err != nil {
		return errors.Annotate(err, "while serializing args")
	}

	req.Header.Set("Content-Type", CTYPE_JSON)
	req.Body = ioutil.NopCloser(bytes.NewBuffer(data))
	return nil
}
