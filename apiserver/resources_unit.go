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

	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// UnitResourcesHandler is the HTTP handler for unit agent downloads of
// resources.
type UnitResourcesHandler struct {
	NewOpener func(*http.Request, ...string) (resource.Opener, state.PoolHelper, error)
}

// ServeHTTP implements http.Handler.
func (h *UnitResourcesHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		opener, ph, err := h.NewOpener(req, names.UnitTagKind, names.ApplicationTagKind)
		if err != nil {
			if err := sendError(resp, err); err != nil {
				logger.Errorf(context.TODO(), "%v", err)
			}
			return
		}
		defer ph.Release()

		name := req.URL.Query().Get(":resource")
		opened, err := opener.OpenResource(req.Context(), name)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				// non internal errors is not real errors.
				logger.Warningf(context.TODO(), "cannot fetch resource reader: %v", err)
			} else {
				logger.Errorf(context.TODO(), "cannot fetch resource reader: %v", err)
			}
			if err := sendError(resp, err); err != nil {
				logger.Errorf(context.TODO(), "%v", err)
			}
			return
		}
		defer opened.Close()

		hdr := resp.Header()
		hdr.Set("Content-Type", params.ContentTypeRaw)
		hdr.Set("Content-Length", fmt.Sprint(opened.Size))
		hdr.Set("Content-Sha384", opened.Fingerprint.String())

		resp.WriteHeader(http.StatusOK)
		if _, err := io.Copy(resp, opened); err != nil {
			// We cannot use SendHTTPError here, so we log the error
			// and move on.
			logger.Errorf(context.TODO(), "unable to complete stream for resource: %v", err)
			return
		}

		// Mark the downloaded resource as in use on the unit.
		err = opener.SetResourceUsed(req.Context(), name)
		if err != nil {
			logger.Errorf(context.TODO(), "setting resource %s as in use: %w", name, err)
		}
	default:
		if err := sendError(resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method)); err != nil {
			logger.Errorf(context.TODO(), "%v", err)
		}
	}
}
