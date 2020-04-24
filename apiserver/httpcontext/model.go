// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/names/v4"
)

// ImpliedModelHandler is an http.Handler that associates requests that
// it handles with a specified model UUID. The model UUID can then be
// extracted using the RequestModel function in this package.
type ImpliedModelHandler struct {
	http.Handler
	ModelUUID string
}

// ServeHTTP is part of the http.Handler interface.
func (h *ImpliedModelHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := context.WithValue(req.Context(), modelKey{}, h.ModelUUID)
	req = req.WithContext(ctx)
	h.Handler.ServeHTTP(w, req)
}

// QueryModelHandler is an http.Handler that associates requests that
// it handles with a model UUID extracted from a specified query parameter.
// The model UUID can then be extracted using the RequestModel function
// in this package.
type QueryModelHandler struct {
	http.Handler
	Query string
}

// ServeHTTP is part of the http.Handler interface.
func (h *QueryModelHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	modelUUID := req.URL.Query().Get(h.Query)
	if modelUUID != "" {
		if !names.IsValidModel(modelUUID) {
			http.Error(w,
				fmt.Sprintf("invalid model UUID %q", modelUUID),
				http.StatusBadRequest,
			)
			return
		}
		ctx := context.WithValue(req.Context(), modelKey{}, modelUUID)
		req = req.WithContext(ctx)
	}
	h.Handler.ServeHTTP(w, req)
}

type modelKey struct{}

// RequestModelUUID returns the model UUID associated with this request
// if there is one, or the empty string otherwise. No attempt is made
// to validate the model UUID; QueryModelHandler does this, and
// ImpliedModelHandler should always be supplied with a valid UUID.
func RequestModelUUID(req *http.Request) string {
	if value := req.Context().Value(modelKey{}); value != nil {
		return value.(string)
	}
	return ""
}
