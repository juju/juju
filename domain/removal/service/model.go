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
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// ModelState describes methods for interacting with model state.
type ModelState interface {
	// ModelExists returns true if a model exists with the input model
	// UUID.
	ModelExists(ctx context.Context, modelUUID string) (bool, error)

	// EnsureModelNotAliveCascade ensures that there is no model identified
	// by the input model UUID, that is still alive.
	EnsureModelNotAliveCascade(ctx context.Context, modelUUID string, force bool) (removal.ModelArtifacts, error)

	// ModelScheduleRemoval schedules a removal job for the model with the
	// input UUID, qualified with the input force boolean.
	// We don't care if the unit does not exist at this point because:
	// - it should have been validated prior to calling this method,
	// - the removal job executor will handle that fact.
	ModelScheduleRemoval(
		ctx context.Context,
		removalDeadUUID, removalDeleteUUID, modelUUID string,
		force bool, when time.Time,
	) error

	// GetModelLife retrieves the life state of a model.
	GetModelLife(ctx context.Context, modelUUID string) (life.Life, error)

	// MarkModelAsDead marks the model with the input UUID as dead.
	MarkModelAsDead(ctx context.Context, modelUUID string) error

	// DeleteModelArtifacts deletes all artifacts associated with a model.
	DeleteModelArtifacts(ctx context.Context, modelUUID string) error
}

// RemoveModel checks if a model with the input name exists.
// If it does, the model is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// model. This duration is ignored if the force argument is false.
// The UUID for the scheduled removal job is returned.
func (s *Service) RemoveModel(
	ctx context.Context,
	modelUUID model.UUID,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// We have to perform the following steps in the following order, to ensure
	// that we can be reentrant during the removal process:
	// 1. Check the model exists in the controller database. If it does not,
	//    then log that fact and continue onwards. The model might have been
	//    removed in the controller database, but still exist in the
	//    model database.
	// 2. Cascade the model removal in the controller database by setting it
	//    to "dying", any associated artifacts will also be set to "dying".
	// 3. Check the model exists in the model database. If it does not, and
	//    the controller model doesn't exist, then we can return early.
	// 4. Ensure the model is not alive in the model database and return any
	//    artifacts that were transitioned from alive to dying.
	// 5. Schedule the model removal job in the model database.
	// 6. If there are any relations, units, machines or applications that
	//    were transitioned from alive to dying, schedule their removal
	//    as well.

	controllerModelExists, err := s.controllerState.ModelExists(ctx, modelUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if controller model exists: %w", err)
	} else if !controllerModelExists {
		s.logger.Infof(ctx, "model %q does not exist in controller database", modelUUID)
	}

	// If the model doesn't exist we can still run this, it just will be a
	// no-op.
	if err := s.controllerState.EnsureModelNotAliveCascade(ctx, modelUUID.String(), force); err != nil {
		return "", errors.Errorf("ensuring model %q is not alive: %w", modelUUID, err)
	}

	// Now check that the model exists in the model database. If it doesn't
	// exist and the controller model exists, then we can return early. We've
	// successfully removed the model from the database.
	modelExists, err := s.modelState.ModelExists(ctx, modelUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if model exists: %w", err)
	} else if !modelExists && !controllerModelExists {
		return "", errors.Errorf("model does not exist").Add(modelerrors.NotFound)
	}

	// Either the model in the controller database or the model database exists,
	// so we can proceed with the removal.
	artifacts, err := s.modelState.EnsureModelNotAliveCascade(ctx, modelUUID.String(), force)
	if err != nil {
		return "", errors.Errorf("model %q: %w", modelUUID, err)
	}

	// From here on, we can assume that the model and any associated model
	// artifacts (machines, applications, units, etc) are not alive.

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the unit if the normal destruction
			// workflows complete within the the wait duration.
			if _, _, err := s.modelScheduleRemoval(ctx, modelUUID, false, 0); err != nil {
				return "", errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal")
			wait = 0
		}
	}

	modelJobUUID, _, err := s.modelScheduleRemoval(ctx, modelUUID, force, wait)
	if err != nil {
		return "", errors.Capture(err)
	} else if artifacts.Empty() {
		// If there are no units or models to update, we can return early.
		return modelJobUUID, nil
	}

	if len(artifacts.RelationUUIDs) > 0 {
		// If there are any relations that transitioned from alive to dying or
		// dead, we need to schedule their removal as well.
		s.logger.Infof(ctx, "model has relations %v, scheduling removal", artifacts.RelationUUIDs)

		s.removeRelations(ctx, artifacts.RelationUUIDs, force, wait)
	}

	if len(artifacts.UnitUUIDs) > 0 {
		// If there are any units that transitioned from alive to dying or
		// dead, we need to schedule their removal as well.
		s.logger.Infof(ctx, "model has units %v, scheduling removal", artifacts.UnitUUIDs)

		s.removeUnits(ctx, artifacts.UnitUUIDs, force, wait)
	}

	if len(artifacts.MachineUUIDs) > 0 {
		// If there are any machines that transitioned from alive to dying or
		// dead, we need to schedule their removal as well.
		s.logger.Infof(ctx, "model has machines %v, scheduling removal", artifacts.MachineUUIDs)

		s.removeMachines(ctx, artifacts.MachineUUIDs, force, wait)
	}

	if len(artifacts.ApplicationUUIDs) > 0 {
		// If there are any applications that transitioned from alive to dying
		// or dead, we need to schedule their removal as well.
		s.logger.Infof(ctx, "model has applications %v, scheduling removal", artifacts.ApplicationUUIDs)

		s.removeApplications(ctx, artifacts.ApplicationUUIDs, force, wait)
	}

	return modelJobUUID, nil
}

func (s *Service) modelScheduleRemoval(
	ctx context.Context, modelUUID model.UUID, force bool, wait time.Duration,
) (removal.UUID, removal.UUID, error) {
	jobDeadUUID, err := removal.NewUUID()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	jobDeleteUUID, err := removal.NewUUID()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	if err := s.modelState.ModelScheduleRemoval(
		ctx,
		jobDeadUUID.String(), jobDeleteUUID.String(),
		modelUUID.String(),
		force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", "", errors.Errorf("model: %w", err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q and %q", jobDeadUUID, jobDeleteUUID)
	return jobDeadUUID, jobDeleteUUID, nil
}

// processModelDeadJob sets the model to dead if it meets the requirements.
// Note that we do not need transactionality here:
//   - Life can only advance - it cannot become alive if dying or dead.
//   - All artifacts associated with the model will also have to be removed.
func (s *Service) processModelDeadJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.ModelDeadJob {
		return errors.Errorf("job type: %q not valid for model removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	controllerModelExists, err := s.controllerState.ModelExists(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("checking if controller model %q exists: %w", job.EntityUUID, err)
	}

	modelLife, err := s.modelState.GetModelLife(ctx, job.EntityUUID)
	if errors.Is(err, modelerrors.NotFound) && !controllerModelExists {
		// This is a programming error, as we should always have a model if
		// we have a job for it.
		return errors.Errorf("model %q not found for removal job %q", job.EntityUUID, job.UUID).Add(
			removalerrors.RemovalModelRemoved)
	} else if err != nil && !errors.Is(err, modelerrors.NotFound) {
		return errors.Errorf("getting model %q life: %w", job.EntityUUID, err)
	}

	if modelLife == life.Alive {
		return errors.Errorf("model %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	if err := s.modelState.MarkModelAsDead(ctx, job.EntityUUID); err != nil && !errors.Is(err, modelerrors.NotFound) {
		return errors.Errorf("marking model %q as dead: %w", job.EntityUUID, err)
	}

	if err := s.controllerState.MarkModelAsDead(ctx, job.EntityUUID); err != nil && !errors.Is(err, modelerrors.NotFound) {
		return errors.Errorf("marking controller model %q as dead: %w", job.EntityUUID, err)
	}

	s.logger.Infof(ctx, "model %q marked as dead", job.EntityUUID)

	return nil
}
