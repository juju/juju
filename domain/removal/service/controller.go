// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
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
	DeleteModel(ctx context.Context, modelUUID string, force bool) error
}

// RemoveController sets the controller model to dying and returns the
// hosted model UUIDs that will also need to be scheduled for removal.
// This does not cascade the removal of any other artifacts in the model
// directly. That is done when all models are completely removed and the
// controller model will be scheduled for removal then. This prevents the
// controller model from being removed before all hosted models are removed.
func (s *Service) RemoveController(
	ctx context.Context,
	force bool,
	wait time.Duration,
) ([]model.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Ensure that we're actually the controller model, otherwise the flow
	// cannot proceed.
	if ok, err := s.modelState.IsControllerModel(ctx, s.modelUUID.String()); errors.Is(err, modelerrors.NotFound) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	} else if !ok {
		return nil, errors.Errorf("model %q is not the controller model", s.modelUUID)
	}

	// Get all the hosted models in the controller, the result can then be
	// iterated over to ensure that they are not alive.
	modelUUIDs, err := s.controllerState.GetModelUUIDs(ctx)
	if err != nil {
		return nil, errors.Capture(err)
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

	// We're the controller model, so we can proceed with the removal.
	if _, err := s.removeControllerModel(ctx, force, wait); err != nil {
		return filteredModelUUIDs, errors.Capture(err)
	}

	return filteredModelUUIDs, nil
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

// processControllerModelJob waits until all hosted models are removed from the
// controller database. It will then schedule the removal job for the controller
// model.
func (s *Service) processControllerModelJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.ControllerModelJob {
		return errors.Errorf("job type: %q not valid for controller model removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	modelUUIDs, err := s.controllerState.GetModelUUIDs(ctx)
	if err != nil {
		return errors.Errorf("retrieving model UUIDs from controller: %w", err)
	}

	remnants := set.NewStrings(modelUUIDs...)
	remnants.Remove(job.EntityUUID)
	if remnants.Size() > 0 {
		// There are still models in the controller, so we need to wait
		// until they are all removed.
		return errors.Errorf("controller model %q removal job %q waiting for hosted models to be removed",
			job.EntityUUID, job.UUID).Add(removalerrors.RemovalJobIncomplete)
	}

	// We've safely removed all models, we can now schedule the removal
	// of the controller model.

	// This is the only place where we should schedule the removal job within
	// another removal job, otherwise we could end up in a non-deterministic
	// loop.

	removalUUID, err := s.removeModel(ctx, model.UUID(job.EntityUUID), job.Force, job.ScheduledFor.Sub(s.clock.Now().UTC()))
	if errors.Is(err, modelerrors.NotFound) {
		return errors.Errorf("controller model %q does not exist, removal job %q complete", job.EntityUUID, job.UUID).
			Add(removalerrors.RemovalModelRemoved)
	} else if err != nil {
		return errors.Errorf("scheduling removal of controller model %q: %w", job.EntityUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for controller model %q from controller model removal job %q",
		removalUUID, job.EntityUUID, job.UUID)

	return nil
}
