// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

// ControllerDBState describes retrieval and persistence methods for entity
// removal in the controller database.
type ControllerState interface {
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
// controller model to dying and then will set all models to dying. This should
// also ensure that no new models can be created in the controller if it is set
// to dying.
//
// The removal of the controller will always use force as we need to remove the
// controller model. Passing in force will remove all entities from the mortal
// coil and not do any proper clean up. Instead, we need to first attempt a
// graceful removal, then it can be called again, which will check the
// controller model life state. If it is already dying, which will indicate
// that a previous removal attempt was made, then we can set force to true and
// use the maxWait time.
// The maxWait will always use the new input value, which will be used when
// removing the controller model.
func (s *Service) RemoveController(
	ctx context.Context,
	wait time.Duration,
) ([]model.UUID, bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Get all the models in the controller, we can then iterate through
	// them and set them to dying.
	modelUUIDs, err := s.controllerState.GetModelUUIDs(ctx)
	if err != nil {
		return nil, false, errors.Capture(err)
	}

	var filteredModelUUIDs []model.UUID
	for _, mUUID := range modelUUIDs {
		typedUUID := model.UUID(mUUID)
		if typedUUID == s.modelUUID {
			// Don't include the controller model in the returned slice.
			continue
		}
		filteredModelUUIDs = append(filteredModelUUIDs, typedUUID)
	}

	if ok, err := s.modelState.IsControllerModel(ctx, s.modelUUID.String()); errors.Is(err, modelerrors.NotFound) {
		return filteredModelUUIDs, true, nil
	} else if err != nil {
		return nil, false, errors.Capture(err)
	} else if !ok {
		return nil, false, errors.Errorf("model %q is not the controller model", s.modelUUID)
	}

	// If the controller model is already dying, we can set force to true and
	// use the maxWait time.
	var modelForce bool
	if lifeState, err := s.modelState.GetModelLife(ctx, s.modelUUID.String()); err != nil {
		return nil, false, errors.Capture(err)
	} else if lifeState != life.Alive {
		modelForce = true
	}

	// We're the controller model, so we can proceed with the removal.
	if _, err := s.removeModel(ctx, s.modelUUID, modelForce, wait); err != nil {
		return nil, false, errors.Capture(err)
	}

	return filteredModelUUIDs, modelForce, nil
}
