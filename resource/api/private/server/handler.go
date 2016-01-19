// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

// TODO(ericsnow) Eliminate the apiserver dependencies, if possible.

import (
	"io"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// TODO(ericsnow) Define the HTTPHandlerConstraints here? Perhaps
// even the HTTPHandlerSpec?

// LegacyHTTPHandler is the HTTP handler for the resources
// endpoint. We use it rather having a separate handler for each HTTP
// method since registered API handlers must handle *all* HTTP methods
// currently.
type LegacyHTTPHandler struct {
	// Connect opens a connection to state resources.
	Connect func(*http.Request) (UnitDataStore, error)

	// HandleDownload provides the download functionality.
	HandleDownload func(st UnitDataStore, req *http.Request) (resource.Resource, io.ReadCloser, error)
}

// NewLegacyHTTPHandler creates a new http.Handler for the resources endpoint.
func NewLegacyHTTPHandler(connect func(*http.Request) (UnitDataStore, error)) *LegacyHTTPHandler {
	return &LegacyHTTPHandler{
		Connect:        connect,
		HandleDownload: handleDownload,
	}
}

// ServeHTTP implements http.Handler.
func (h *LegacyHTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	st, err := h.Connect(req)
	if err != nil {
		api.SendHTTPError(resp, err)
		return
	}

	// We do this *after* authorization, etc. (in h.Connect) in order
	// to prioritize errors that may originate there.
	switch req.Method {
	case "GET":
		logger.Infof("handling resource download request")
		resource, resourceReader, err := h.HandleDownload(st, req)
		if err != nil {
			logger.Errorf("cannot fetch resource reader: %v", err)
			api.SendHTTPError(resp, err)
			return
		}
		defer resourceReader.Close()

		api.UpdateDownloadResponse(resp, resource)

		resp.WriteHeader(http.StatusOK)
		if _, err := io.Copy(resp, resourceReader); err != nil {
			logger.Errorf("unable to complete stream for resource: %v", err)
		} else {
			logger.Infof("resource download request successful")
		}
	default:
		api.SendHTTPError(resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
	}
}
