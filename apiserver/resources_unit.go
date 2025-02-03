// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/rpc/params"
)

type ResourceOpenerGetter interface {
	Opener(*http.Request, ...string) (coreresource.Opener, error)
}

// UnitResourcesHandler is the HTTP handler for unit agent downloads of
// resources.
type UnitResourcesHandler struct {
	resourceOpenerGetter ResourceOpenerGetter
}

// NewUnitResourcesHandler returns a new HTTP handler for unit agent downloads
// of resources.
func NewUnitResourcesHandler(
	resourceOpenerGetter ResourceOpenerGetter,
) *UnitResourcesHandler {
	return &UnitResourcesHandler{
		resourceOpenerGetter: resourceOpenerGetter,
	}
}

// ServeHTTP implements http.Handler.
func (h *UnitResourcesHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		if err := h.serveGet(resp, req); err != nil {
			if err := sendError(resp, err); err != nil {
				logger.Errorf(context.TODO(), "%v", err)
			}
		}
	default:
		if err := sendError(
			resp,
			errors.MethodNotAllowedf("unsupported method: %q", req.Method),
		); err != nil {
			logger.Errorf(context.TODO(), "%v", err)
		}
	}
}

func (h *UnitResourcesHandler) serveGet(resp http.ResponseWriter, req *http.Request) error {
	opener, err := h.resourceOpenerGetter.Opener(req, names.UnitTagKind, names.ApplicationTagKind)
	if err != nil {
		return err
	}

	name := req.URL.Query().Get(":resource")
	opened, err := opener.OpenResource(req.Context(), name)
	if err != nil {
		logger.Errorf(context.TODO(), "cannot fetch resource reader: %v", err)
		return err
	}
	defer opened.Close()

	hdr := resp.Header()
	hdr.Set("Content-Type", params.ContentTypeRaw)
	hdr.Set("Content-Length", fmt.Sprint(opened.Size))
	hdr.Set("Content-Sha384", opened.Fingerprint.String())

	resp.WriteHeader(http.StatusOK)
	var bytesWritten int64
	if bytesWritten, err = io.Copy(resp, opened); err != nil {
		// We cannot use SendHTTPError here, so we log the error and move on.
		logger.Warningf(context.TODO(), "unable to complete stream for resource, %d bytes streamed: %v out of %v",
			bytesWritten, opened.Size, err)
		return nil
	}

	if bytesWritten != opened.Size {
		logger.Warningf(context.TODO(), "resource streamed to unit had unexpected size: got %v, expected %v",
			bytesWritten, opened.Size)
	}

	// Mark the downloaded resource as in use on the unit.
	err = opener.SetResourceUsed(req.Context(), name)
	if err != nil {
		logger.Warningf(context.TODO(), "setting resource %s as in use: %w", name, err)
	}

	return nil
}
