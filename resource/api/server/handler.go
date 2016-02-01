// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/resource/api"
)

// TODO(ericsnow) Define the HTTPHandlerConstraints here? Perhaps
// even the HTTPHandlerSpec?

// LegacyHTTPHandler is the HTTP handler for the resources endpoint. We
// use it rather having a separate handler for each HTTP method since
// registered API handlers must handle *all* HTTP methods currently.
type LegacyHTTPHandler struct {
	// Connect opens a connection to state resources.
	Connect func(*http.Request) (DataStore, names.Tag, error)

	// HandleUpload provides the upload functionality.
	HandleUpload func(username string, st DataStore, req *http.Request) (*api.UploadResult, error)
}

// TODO(ericsnow) Can username be extracted from the request?

// NewLegacyHTTPHandler creates a new http.Handler for the resources endpoint.
func NewLegacyHTTPHandler(connect func(*http.Request) (DataStore, names.Tag, error)) *LegacyHTTPHandler {
	return &LegacyHTTPHandler{
		Connect: connect,
		HandleUpload: func(username string, st DataStore, req *http.Request) (*api.UploadResult, error) {
			uh := UploadHandler{
				Username: username,
				Store:    st,
			}
			return uh.HandleRequest(req)
		},
	}
}

// ServeHTTP implements http.Handler.
func (h *LegacyHTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	st, tag, err := h.Connect(req)
	if err != nil {
		api.SendHTTPError(resp, err)
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
		response, err := h.HandleUpload(username, st, req)
		if err != nil {
			api.SendHTTPError(resp, err)
			return
		}
		api.SendHTTPStatusAndJSON(resp, http.StatusOK, &response)
		logger.Infof("resource upload request successful")
	default:
		api.SendHTTPError(resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
	}
}
