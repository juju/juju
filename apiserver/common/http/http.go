// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) Move this to its own package or even to another repo?

package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.apiserver.common.http")

const (
	// The values used for content-type in juju's direct HTTP code:

	// ContentTypeJSON is the HTTP content-type value used for JSON content.
	ContentTypeJSON = "application/json"

	// ContentTypeRaw is the HTTP content-type value used for raw, unformattedcontent.
	ContentTypeRaw = "application/octet-stream"
)

// NormalizePath cleans up the provided URL path and makes it absolute.
func NormalizePath(pth string) string {
	return path.Clean(path.Join("/", pth))
}

// TODO(ericsnow) This is copied from apiserver/httpcontext.go...

// sendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func sendStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) {
	body, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("cannot marshal JSON result %#v: %v", response, err)
		return
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	w.Write(body)
}

// TODO(ericsnow) This is copied from apiserver/common/errors.go...

func serverErrorAndStatus(err error) (apiErr error, status int) {
	if err == nil {
		return nil, http.StatusOK
	}

	var code string
	switch {
	case errors.IsMethodNotAllowed(err):
		status = http.StatusMethodNotAllowed
		code = params.CodeMethodNotAllowed
	default:
		status = http.StatusInternalServerError
	}
	apiErr = &params.Error{
		Message: err.Error(),
		Code:    code,
	}
	return apiErr, status
}
