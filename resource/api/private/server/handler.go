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

// NewLegacyHTTPHandler creates a new http.Handler for the resources endpoint.
func NewLegacyHTTPHandler(deps LegacyHTTPHandlerDeps) *LegacyHTTPHandler {
	return &LegacyHTTPHandler{
		LegacyHTTPHandlerDeps: deps,
	}
}

// ServeHTTP implements http.Handler.
func (h *LegacyHTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	opener, err := h.NewResourceOpener(req)
	if err != nil {
		h.SendHTTPError(resp, err)
		return
	}

	// We do this *after* authorization, etc. (in h.Extract...) in order
	// to prioritize errors that may originate there.
	switch req.Method {
	case "GET":
		logger.Infof("handling resource download request")

		opened, err := h.HandleDownload(opener, req)
		if err != nil {
			logger.Errorf("cannot fetch resource reader: %v", err)
			h.SendHTTPError(resp, err)
			return
		}
		defer opened.Close()

		h.UpdateDownloadResponse(resp, opened.Resource)

		resp.WriteHeader(http.StatusOK)
		if err := h.Copy(resp, opened); err != nil {
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

// LegacyHTTPHandlerDeps exposes the external dependencies
// of LegacyHTTPHandler.
type LegacyHTTPHandlerDeps interface {
	baseLegacyHTTPHandlerDeps
	ExtraDeps
}

//ExtraDeps exposes the non-superficial dependencies of LegacyHTTPHandler.
type ExtraDeps interface {
	// NewResourceOpener returns a new opener for the request.
	NewResourceOpener(*http.Request) (resource.Opener, error)
}

type baseLegacyHTTPHandlerDeps interface {
	// UpdateDownloadResponse updates the HTTP response with the info
	// from the resource.
	UpdateDownloadResponse(http.ResponseWriter, resource.Resource)

	// SendHTTPError wraps the error in an API error and writes it to the response.
	SendHTTPError(http.ResponseWriter, error)

	// HandleDownload provides the download functionality.
	HandleDownload(resource.Opener, *http.Request) (resource.Opened, error)

	// Copy implements the functionality of io.Copy().
	Copy(io.Writer, io.Reader) error
}

// NewLegacyHTTPHandlerDeps returns an implementation of LegacyHTTPHandlerDeps.
func NewLegacyHTTPHandlerDeps(extraDeps ExtraDeps) LegacyHTTPHandlerDeps {
	return &legacyHTTPHandlerDeps{
		ExtraDeps: extraDeps,
	}
}

// legacyHTTPHandlerDeps is a partial implementation of LegacyHandlerDeps.
type legacyHTTPHandlerDeps struct {
	ExtraDeps
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
func (deps legacyHTTPHandlerDeps) HandleDownload(opener resource.Opener, req *http.Request) (resource.Opened, error) {
	name := api.ExtractDownloadRequest(req)
	return opener.OpenResource(name)
}

// Copy implements LegacyHTTPHandlerDeps.
func (deps legacyHTTPHandlerDeps) Copy(w io.Writer, r io.Reader) error {
	_, err := io.Copy(w, r)
	return err
}
