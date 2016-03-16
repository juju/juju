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

// UnitResourceHandler is the HTTP handler for the resources
// endpoint. We use it rather having a separate handler for each HTTP
// method since registered API handlers must handle *all* HTTP methods
// currently.
type UnitResourceHandler struct {
	UnitResourceHandlerDeps
}

// NewUnitResourceHandler creates a new http.Handler for the resources endpoint.
func NewUnitResourceHandler(deps UnitResourceHandlerDeps) *UnitResourceHandler {
	return &UnitResourceHandler{
		UnitResourceHandlerDeps: deps,
	}
}

// ServeHTTP implements http.Handler.
func (h *UnitResourceHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
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

// UnitResourceHandlerDeps exposes the external dependencies
// of UnitResourceHandler.
type UnitResourceHandlerDeps interface {
	baseUnitResourceHandlerDeps
	ExtraDeps
}

//ExtraDeps exposes the non-superficial dependencies of UnitResourceHandler.
type ExtraDeps interface {
	// NewResourceOpener returns a new opener for the request.
	NewResourceOpener(*http.Request) (resource.Opener, error)
}

type baseUnitResourceHandlerDeps interface {
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

// NewUnitResourceHandlerDeps returns an implementation of UnitResourceHandlerDeps.
func NewUnitResourceHandlerDeps(extraDeps ExtraDeps) UnitResourceHandlerDeps {
	return &unitResourceHandlerDeps{
		ExtraDeps: extraDeps,
	}
}

// unitResourceHandlerDeps is a partial implementation of LegacyHandlerDeps.
type unitResourceHandlerDeps struct {
	ExtraDeps
}

// SendHTTPError implements UnitResourceHandlerDeps.
func (deps unitResourceHandlerDeps) SendHTTPError(resp http.ResponseWriter, err error) {
	api.SendHTTPError(resp, err)
}

// UpdateDownloadResponse implements UnitResourceHandlerDeps.
func (deps unitResourceHandlerDeps) UpdateDownloadResponse(resp http.ResponseWriter, info resource.Resource) {
	api.UpdateDownloadResponse(resp, info)
}

// HandleDownload implements UnitResourceHandlerDeps.
func (deps unitResourceHandlerDeps) HandleDownload(opener resource.Opener, req *http.Request) (resource.Opened, error) {
	name := api.ExtractDownloadRequest(req)
	return opener.OpenResource(name)
}

// Copy implements UnitResourceHandlerDeps.
func (deps unitResourceHandlerDeps) Copy(w io.Writer, r io.Reader) error {
	_, err := io.Copy(w, r)
	return err
}
