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
	LegacyHTTPHandlerDeps
}

// LegacyHTTPHandlerDeps exposes the external dependencies
// of LegacyHTTPHandler.
type LegacyHTTPHandlerDeps interface {
	// Connect opens a connection to state resources.
	Connect(*http.Request) (UnitDataStore, error)

	// UpdateDownloadResponse updates the HTTP response with the info
	// from the resource.
	UpdateDownloadResponse(http.ResponseWriter, resource.Resource)

	// SendHTTPError wraps the error in an API error and writes it to the response.
	SendHTTPError(http.ResponseWriter, error)

	// HandleDownload provides the download functionality.
	HandleDownload(UnitDataStore, *http.Request) (resource.Resource, io.ReadCloser, error)

	// Copy implements the functionality of io.Copy().
	Copy(io.Writer, io.Reader) error
}

// NewLegacyHTTPHandler creates a new http.Handler for the resources endpoint.
func NewLegacyHTTPHandler(connect func(*http.Request) (UnitDataStore, error)) *LegacyHTTPHandler {
	deps := &legacyHTTPHandlerDeps{
		connect: connect,
	}
	return &LegacyHTTPHandler{
		LegacyHTTPHandlerDeps: deps,
	}
}

// ServeHTTP implements http.Handler.
func (h *LegacyHTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	st, err := h.Connect(req)
	if err != nil {
		h.SendHTTPError(resp, err)
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
			h.SendHTTPError(resp, err)
			return
		}
		defer resourceReader.Close()

		h.UpdateDownloadResponse(resp, resource)

		resp.WriteHeader(http.StatusOK)
		if err := h.Copy(resp, resourceReader); err != nil {
			// We cannot use api.SendHTTPError here, so we log the error
			// and move on.
			logger.Errorf("unable to complete stream for resource: %v", err)
			return
		}

		logger.Infof("resource download request successful")
	default:
		h.SendHTTPError(resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
	}
}

type legacyHTTPHandlerDeps struct {
	connect func(*http.Request) (UnitDataStore, error)
}

// Connect implements LegacyHTTPHandlerDeps.
func (deps legacyHTTPHandlerDeps) Connect(req *http.Request) (UnitDataStore, error) {
	return deps.connect(req)
}

// SendHTTPError implements LegacyHTTPHandlerDeps.
func (deps legacyHTTPHandlerDeps) SendHTTPError(resp http.ResponseWriter, err error) {
	api.SendHTTPError(resp, err)
}

// UpdateDownloadResponse implements LegacyHTTPHandlerDeps.
func (deps legacyHTTPHandlerDeps) UpdateDownloadResponse(resp http.ResponseWriter, info resource.Resource) {
	api.UpdateDownloadResponse(resp, info)
}

// HandleDownload implements LegacyHTTPHandlerDeps.
func (deps legacyHTTPHandlerDeps) HandleDownload(st UnitDataStore, req *http.Request) (resource.Resource, io.ReadCloser, error) {
	return HandleDownload(req, handleDownloadDeps{
		DownloadDataStore: st,
	})
}

// Copy implements LegacyHTTPHandlerDeps.
func (deps legacyHTTPHandlerDeps) Copy(w io.Writer, r io.Reader) error {
	_, err := io.Copy(w, r)
	return err
}
