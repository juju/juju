// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

// TODO(ericsnow) Eliminate the apiserver dependencies, if possible.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

// TODO(ericsnow) Define the HTTPHandlerConstraints here? Perhaps
// even the HTTPHandlerSpec?

// LegacyHTTPHandler is the HTTP handler for the resources endpoint. We
// use it rather than wrapping the functions since API HTTP endpoints
// must handle *all* HTTP methods.
type LegacyHTTPHandler struct {
	// Connect opens a connection to state resources.
	Connect func(*http.Request) (DataStore, names.Tag, error)

	// HandleUpload provides the upload functionality.
	HandleUpload func(username string, st DataStore, req *http.Request) error
}

// TODO(ericsnow) Can username be extracted from the request?

// NewLegacyHTTPHandler creates a new http.Handler for the resources endpoint.
func NewLegacyHTTPHandler(connect func(*http.Request) (DataStore, names.Tag, error)) *LegacyHTTPHandler {
	return &LegacyHTTPHandler{
		Connect: connect,
		HandleUpload: func(username string, st DataStore, req *http.Request) error {
			uh := UploadHandler{
				Username:         username,
				Store:            st,
				CurrentTimestamp: time.Now,
			}
			return uh.HandleRequest(req)
		},
	}
}

// ServeHTTP implements http.Handler.
func (h *LegacyHTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	st, tag, err := h.Connect(req)
	if err != nil {
		sendHTTPError(resp, err)
		return
	}

	var username string
	switch tag := tag.(type) {
	case *names.UserTag:
		username = tag.Name()
	default:
		// TODO(ericsnow) Fail?
		username = tag.Id()
	}

	// We do this *after* authorization, etc. (in h.Connect) in order
	// to prioritize errors that may originate there.
	switch req.Method {
	case "PUT":
		logger.Infof("handling resource upload request")
		if err := h.HandleUpload(username, st, req); err != nil {
			sendHTTPError(resp, err)
			return
		}

		// TODO(ericsnow) Clean this up.
		body := "success"
		resp.Header().Set("Content-Type", "text/plain")
		resp.Header().Set("Content-Length", fmt.Sprint(len(body)))
		resp.WriteHeader(http.StatusOK)
		resp.Write([]byte(body))

		logger.Infof("resource upload request successful")
	default:
		sendHTTPError(resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
	}
}

// TODO(ericsnow) There are copied from apiserver/httpcontext.go...

// sendHTTPError sends a JSON-encoded error response
// for errors encountered during processing.
func sendHTTPError(w http.ResponseWriter, err error) {
	err1, statusCode := common.ServerErrorAndStatus(err)
	logger.Debugf("sending error: %d %v", statusCode, err1)
	sendHTTPStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: err1,
	})
}

// sendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func sendHTTPStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) {
	body, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("cannot marshal JSON result %#v: %v", response, err)
		return
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	w.Write(body)
}
