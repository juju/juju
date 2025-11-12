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
	"github.com/juju/juju/domain/removal"
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

	// EnsureModelNotAlive ensures that there is no model identified
	// by the input model UUID, that is still alive. This does not cascade
	// all entities associated with the model will still be alive.
	EnsureModelNotAlive(ctx context.Context, modelUUID string, force bool) error

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
	if _, err := s.removeControllerModel(ctx, modelForce, wait); err != nil {
		return nil, false, errors.Capture(err)
	}

	return filteredModelUUIDs, modelForce, nil
}

// removeControllerModel sets the controller model to dying, this does not
// cascade the removal of any other artifacts in the model directly. That is
// done when all models are completely removed.
func (s *Service) removeControllerModel(
	ctx context.Context,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	controllerModelExists, err := s.controllerState.ModelExists(ctx, s.modelUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if controller model exists: %w", err)
	} else if !controllerModelExists {
		s.logger.Infof(ctx, "model %q does not exist in controller database", s.modelUUID)
	}

	// If the model doesn't exist we can still run this, it just will be a
	// no-op.
	if err := s.controllerState.EnsureModelNotAlive(ctx, s.modelUUID.String(), force); err != nil {
		return "", errors.Errorf("ensuring controller model %q is not alive: %w", s.modelUUID, err)
	}

	// Now check that the controller model exists in the model database. If it
	// doesn't exist and the model in the controller database exists, then we
	// can return early. We've successfully removed the model from the database.
	modelExists, err := s.modelState.ModelExists(ctx, s.modelUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if model exists: %w", err)
	} else if !modelExists && !controllerModelExists {
		return "", errors.Errorf("model does not exist").Add(modelerrors.NotFound)
	}

	// Either the model in the controller database or the model database exists,
	// so we can proceed with the removal.
	if err := s.modelState.EnsureModelNotAlive(ctx, s.modelUUID.String(), force); err != nil {
		return "", errors.Errorf("model %q: %w", s.modelUUID, err)
	}

	// From here on, we can assume that the model and any associated model
	// artifacts (machines, applications, units, etc) are not alive.

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the unit if the normal destruction
			// workflows complete within the the wait duration.
			if _, err := s.controllerModelScheduleRemoval(ctx, s.modelUUID, false, 0); err != nil {
				return "", errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal")
			wait = 0
		}
	}

	modelJobUUID, err := s.controllerModelScheduleRemoval(ctx, s.modelUUID, force, wait)
	if err != nil {
		return "", errors.Capture(err)
	}

	return modelJobUUID, nil
}

func (s *Service) controllerModelScheduleRemoval(
	ctx context.Context, modelUUID model.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.modelState.ControllerModelScheduleRemoval(
		ctx,
		jobUUID.String(),
		modelUUID.String(),
		force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("controller model: %w", err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for controller model %q", jobUUID, modelUUID)
	return jobUUID, nil
}
