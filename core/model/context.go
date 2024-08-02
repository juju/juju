// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import "context"

type contextKey string

const (
	// ContextKeyModelUUID is the key used to store the model UUID in the
	// context.
	ContextKeyModelUUID contextKey = "model-uuid"
)

// WitContextModelUUID returns a new context with the model UUID set.
func WithContextModelUUID(ctx context.Context, modelUUID UUID) context.Context {
	return context.WithValue(ctx, ContextKeyModelUUID, modelUUID)
}

// ModelUUIDFromContext returns the model UUID from the context.
func ModelUUIDFromContext(ctx context.Context) (UUID, bool) {
	modelUUID, ok := ctx.Value(ContextKeyModelUUID).(UUID)
	return modelUUID, ok
}
