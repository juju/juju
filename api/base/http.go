// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// HTTPCaller exposes direct HTTP request functionality for the API.
// This is significant for upload and download of files, which the
// websockets-based RPC does not support.
type HTTPCaller interface {
	// NewHTTPRequest returns a new API-relative HTTP request.  Callers
	// should finish the request (setting headers, body) before passing
	// the request to SendHTTPRequest.
	NewHTTPRequest(method, path string) (*http.Request, error)
	// SendHTTPRequest returns the HTTP response from the API server.
	// The caller is then responsible for handling the response.
	SendHTTPRequest(req *http.Request) (*http.Response, error)
}

// CheckHTTPResponse returns the failure serialized in the response
// body.  If there is no failure (an OK status code), it simply returns
// nil.
func CheckHTTPResponse(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Annotate(err, "while reading HTTP response")
	}

	var failure params.Error
	if resp.Header.Get("Content-Type") == "application/json" {
		if err := json.Unmarshal(body, &failure); err != nil {
			return errors.Annotate(err, "while unserializing the error")
		}
	} else {
		switch resp.StatusCode {
		case http.StatusNotFound:
			fallthrough
		case http.StatusMethodNotAllowed:
			failure.Code = params.CodeNotImplemented
		default:
			// Leave Code empty.
		}

		failure.Message = string(body)
	}
	return &failure
}
