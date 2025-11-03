// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext

import (
	"context"
	"fmt"
	"net/http"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// isControllerModelKey is a context key used to set on a request context if the
// current model being operated on is the controller model.
type isControllerModelKey struct{}

// modelKey is the context key to set the model uuid that is being worked on for
// [context.Context].
type modelKey struct{}

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

// ControllerModelSignalHandler is a [http.Handler] that establishes if the
// current http request is for a model uuid that is considered to be the
// controller model's uuid. This handler is intended to be chained together
// with either [ControllerModelHandler] or [QueryModelHandler].
type ControllerModelSignalHandler struct {
	http.Handler
	ControllerModelUUID coremodel.UUID
}

// ServeHTTP takes a http request and establishes if the model that is the
// subject of the request is for the controller's model. If this is the case
// the handler will set the [isControllerModelKey] on the context and continue
// to pass the request to [ControllerModelSignalHandler.Handler].
//
// It is expected that before this handler is called either the
// [ControllerModelHandler] or [QueryModelHandler] have been run. Should this
// handler not be able to establish if the request is for the controller model
// the key will not be set on the context.
//
// ServeHTTP implements the [http.Handler] interface.
func (h ControllerModelSignalHandler) ServeHTTP(
	w http.ResponseWriter, req *http.Request,
) {
	modelUUID, exists := RequestModelUUID(req.Context())
	if exists && modelUUID == h.ControllerModelUUID.String() {
		// The request is for the controller model so indicate this on the
		// context.
		ctx := SetContextIsControllerModel(req.Context())
		req = req.WithContext(ctx)
	}

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

// MigrationRequestModelUUID returns the model uuid associated with the given
// http request assuming the request is associated with a migration request. If
// no model uuid is found an empty string and false is returned.
func MigrationRequestModelUUID(r *http.Request) (string, bool) {
	modelUUID := r.Header.Get(params.MigrationModelHTTPHeader)
	if modelUUID == "" {
		return "", false
	}
	return modelUUID, true
}

// RequestIsForControllerModel returns true if the provided request context is
// for the controller model. If it can not be established that the request
// context is for the controller model then false will always be returned.
func RequestIsForControllerModel(ctx context.Context) bool {
	if value := ctx.Value(isControllerModelKey{}); value != nil {
		// We don't check the conversion bool here as the default value of bool
		// when casting will be false. This is the behaviour we want.
		val, _ := value.(bool)
		return val
	}
	return false
}

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

// SetContextIsControllerModel sets on the provided context the
// [isControllerModelKey] to true indicating to any further consumers of the
// returned context that the request is for the controller model.
func SetContextIsControllerModel(ctx context.Context) context.Context {
	return context.WithValue(ctx, isControllerModelKey{}, true)
}

// SetContextModelUUID is responsible for taking a model uuid and setting it on
// the supplied context. This is useful for associating a given http request
// context with a model.
func SetContextModelUUID(ctx context.Context, modelUUID coremodel.UUID) context.Context {
	return context.WithValue(ctx, modelKey{}, modelUUID.String())
}
