// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// ControllerDBState describes retrieval and persistence methods for entity
// removal in the controller database.
type ControllerDBState interface {
	// ModelExists returns true if a model exists with the input model
	// UUID.
	ModelExists(ctx context.Context, modelUUID string) (bool, error)

	// GetModelLife retrieves the life state of a model.
	GetModelLife(ctx context.Context, modelUUID string) (life.Life, error)

	// GetModelUUIDs retrieves the UUIDs of all models in the controller.
	GetModelUUIDs(ctx context.Context) ([]string, error)

	// EnsureModelNotAliveCascade ensures that there is no model identified
	// by the input model UUID, that is still alive.
	EnsureModelNotAliveCascade(ctx context.Context, modelUUID string, force bool) error

	// MarkModelAsDead marks the model with the input UUID as dead.
	MarkModelAsDead(ctx context.Context, modelUUID string) error

	// DeleteModel removes the model with the input UUID from the database.
	DeleteModel(ctx context.Context, modelUUID string) error
}

// RemoveController checks if a model is the controller model, and will set the
// controller model to dying and then will set all models to dying. This
// should also ensure that no new models can be created in the controller
// if it is set to dying.
//
// If force is true, the controller will be removed immediately without waiting
// for any ongoing operations to complete. If wait is specified, it will wait
// for the specified duration before proceeding with the removal.
func (s *Service) RemoveController(
	ctx context.Context,
	force bool,
	wait time.Duration,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if ok, err := s.modelState.IsControllerModel(ctx, s.modelUUID.String()); err != nil {
		return errors.Capture(err)
	} else if !ok {
		return errors.Errorf("model %q is not the controller model", s.modelUUID)
	}

	// First get all the models in the controller, we can then iterate through
	// them and set them to dying.
	modelUUIDs, err := s.controllerState.GetModelUUIDs(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// We're the controller model, so we can proceed with the removal.
	if _, err := s.removeModel(ctx, s.modelUUID, force, wait); err != nil {
		return errors.Capture(err)
	}

	for _, modelUUID := range modelUUIDs {
		if _, err := s.removeModel(ctx, model.UUID(modelUUID), force, wait); err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}
