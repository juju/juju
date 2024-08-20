// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/names/v5"
)

// ControllerModelHandler is an http.Handler that associates requests that
// it handles with a specified controller model UUID. The controller model UUID
// can then be extracted using the RequestModelUUID function in this package.
type ControllerModelHandler struct {
	http.Handler
	ControllerModelUUID string
}

// ServeHTTP is part of the http.Handler interface.
func (h *ControllerModelHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := context.WithValue(req.Context(), modelKey{}, h.ControllerModelUUID)
	req = req.WithContext(ctx)
	h.Handler.ServeHTTP(w, req)
}

// QueryModelHandler is an http.Handler that associates requests that
// it handles with a model UUID extracted from a specified query parameter.
// The model UUID can then be extracted using the RequestModelUUID function
// in this package.
type QueryModelHandler struct {
	http.Handler
	Query string
}

// ServeHTTP is part of the http.Handler interface.
func (h *QueryModelHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	modelUUID := req.URL.Query().Get(h.Query)
	validateModelAndServe(h.Handler, modelUUID, w, req)
}

// BucketModelHandler is an http.Handler that associates requests that
// it handles with a model UUID extracted from a specified query parameter that
// must be the objects storage :bucket which is formatted 'model-{modelUUID}'.
// The model UUID can then be extracted using the RequestModelUUID function
// in this package.
type BucketModelHandler struct {
	http.Handler
	Query string
}

// ServeHTTP is part of the http.Handler interface.
func (h *BucketModelHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	modelUUID := req.URL.Query().Get(h.Query)
	validateModelAndServe(h.Handler, modelUUID, w, req)
}

func validateModelAndServe(handler http.Handler, modelUUID string, w http.ResponseWriter, req *http.Request) {
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
	handler.ServeHTTP(w, req)
}

type modelKey struct{}

// RequestModelUUID returns the model UUID associated with this request
// if there is one, or returns false if no valid model UUID is passed. No
// attempt is made to validate the model UUID; QueryModelHandler and
// BucketModelHandler does this, and ControllerModelHandler should always be
// supplied with a valid UUID.
func RequestModelUUID(req *http.Request) (string, bool) {
	if value := req.Context().Value(modelKey{}); value != nil {
		return value.(string), true
	}
	return "", false
}
