// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
)

// HTTPResponse is an HTTP response for use in testing.
type HTTPResponse struct {
	http.Response
	// Buffer is the file underlying Body.
	Buffer bytes.Buffer
}

// NewHTTPResponse returns an HTTP response with an OK status,
// no headers set, and an empty body.
func NewHTTPResponse() *HTTPResponse {
	resp := HTTPResponse{
		Response: http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
		},
	}
	resp.Body = ioutil.NopCloser(&resp.Buffer)
	return &resp
}

// NewErrorResponse returns an HTTP response with the status and
// body set to the provided values.
func NewErrorResponse(statusCode int, msg string) *HTTPResponse {
	resp := NewHTTPResponse()
	resp.StatusCode = statusCode
	if _, err := resp.Buffer.WriteString(msg); err != nil {
		panic(fmt.Sprintf("could not write to buffer: %v", err))
	}
	return resp
}

// NewFailureResponse returns an HTTP response with the status set
// to 500 (Internal Server Error) and the body set to the JSON-encoded
// error.
func NewFailureResponse(failure *params.Error) *HTTPResponse {
	resp := NewHTTPResponse()
	resp.StatusCode = http.StatusInternalServerError
	resp.Header.Set("Content-Type", apihttp.CTypeJSON)
	if err := json.NewEncoder(&resp.Buffer).Encode(failure); err != nil {
		panic(fmt.Sprintf("could not JSON-encode failure: %v", err))
	}
	return resp
}
