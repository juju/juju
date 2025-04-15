// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext

import (
	"context"
	"fmt"
	"net/http"

	coremodel "github.com/juju/juju/core/model"
)

// ControllerModelHandler is an http.Handler that associates requests that
// it handles with a specified controller model UUID. The controller model UUID
// can then be extracted using the RequestModelUUID function in this package.
type ControllerModelHandler struct {
	http.Handler
	ControllerModelUUID coremodel.UUID
}

// ServeHTTP is part of the http.Handler interface.
func (h *ControllerModelHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := SetContextModelUUID(req.Context(), h.ControllerModelUUID)
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

func validateModelAndServe(handler http.Handler, modelUUIDStr string, w http.ResponseWriter, req *http.Request) {
	if modelUUIDStr != "" {
		modelUUID := coremodel.UUID(modelUUIDStr)
		if err := modelUUID.Validate(); err != nil {
			http.Error(w,
				fmt.Sprintf("invalid model UUID %q", modelUUIDStr),
				http.StatusBadRequest,
			)
			return
		}
		ctx := SetContextModelUUID(req.Context(), modelUUID)
		req = req.WithContext(ctx)
	}
	handler.ServeHTTP(w, req)
}

type modelKey struct{}

// RequestModelUUID returns the model UUID associated with the given context
// provided from an httpRequest. No attempt is made to validate the model UUID;
// QueryModelHandler and BucketModelHandler does this, and ControllerModelHandler
// should always be supplied with a valid UUID.
func RequestModelUUID(ctx context.Context) (string, bool) {
	if value := ctx.Value(modelKey{}); value != nil {
		val, ok := value.(string)
		return val, ok
	}
	return "", false
}

// SetContextModelUUID is responsible for taking a model uuid and setting it on
// the supplied context. This is useful for associating a given http request
// context with a model.
func SetContextModelUUID(ctx context.Context, modelUUID coremodel.UUID) context.Context {
	return context.WithValue(ctx, modelKey{}, modelUUID.String())
}
