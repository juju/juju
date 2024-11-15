// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// UnitResourcesHandler is the HTTP handler for unit agent downloads of
// resources.
type UnitResourcesHandler struct {
	NewOpener func(*http.Request, ...string) (resources.Opener, state.PoolHelper, error)
}

// ServeHTTP implements http.Handler.
func (h *UnitResourcesHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	switch req.Method {
	case "GET":
		opener, ph, err := h.NewOpener(req, names.UnitTagKind, names.ApplicationTagKind)
		if err != nil {
			if err := sendError(ctx, resp, err); err != nil {
				logger.Errorf(ctx, "%v", err)
			}
			return
		}
		defer ph.Release()

		name := req.URL.Query().Get(":resource")
		opened, err := opener.OpenResource(req.Context(), name)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				// non internal errors is not real errors.
				logger.Warningf(ctx, "cannot fetch resource reader: %v", err)
			} else {
				logger.Errorf(ctx, "cannot fetch resource reader: %v", err)
			}
			if err := sendError(ctx, resp, err); err != nil {
				logger.Errorf(ctx, "%v", err)
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
			logger.Errorf(ctx, "unable to complete stream for resource: %v", err)
			return
		}
	default:
		if err := sendError(ctx, resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method)); err != nil {
			logger.Errorf(ctx, "%v", err)
		}
	}
}
