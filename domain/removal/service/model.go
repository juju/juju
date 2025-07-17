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
		ctx context.Context, removalUUID, modelUUID string, force bool, when time.Time,
	) error

	// GetModelLife retrieves the life state of a model.
	GetModelLife(ctx context.Context, modelUUID string) (life.Life, error)

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

	exists, err := s.st.ModelExists(ctx, modelUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if model exists: %w", err)
	} else if !exists {
		return "", errors.Errorf("model does not exist").Add(modelerrors.NotFound)
	}

	// Ensure the model is not alive.
	artifacts, err := s.st.EnsureModelNotAliveCascade(ctx, modelUUID.String(), force)
	if err != nil {
		return "", errors.Errorf("model %q: %w", modelUUID, err)
	}

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the unit if the normal destruction
			// workflows complete within the the wait duration.
			if _, err := s.modelScheduleRemoval(ctx, modelUUID, false, 0); err != nil {
				return "", errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal")
			wait = 0
		}
	}

	modelJobUUID, err := s.modelScheduleRemoval(ctx, modelUUID, force, wait)
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
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.st.ModelScheduleRemoval(
		ctx, jobUUID.String(), modelUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("unit: %w", err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q", jobUUID)
	return jobUUID, nil
}

// processModelRemovalJob deletes an model if it is dying.
// Note that we do not need transactionality here:
//   - Life can only advance - it cannot become alive if dying or dead.
func (s *Service) processModelRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.ModelJob {
		return errors.Errorf("job type: %q not valid for model removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.st.GetModelLife(ctx, job.EntityUUID)
	if errors.Is(err, modelerrors.NotFound) {
		// This is a programming error, as we should always have a model if
		// we have a job for it.
		return errors.Errorf("model %q not found for removal job %q", job.EntityUUID, job.UUID).Add(
			removalerrors.RemovalModelRemoved)
	} else if err != nil {
		return errors.Errorf("getting model %q life: %w", job.EntityUUID, err)
	}

	if l == life.Alive {
		return errors.Errorf("model %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	// This will delete any model artifacts that are associated with the model.
	// This will not delete the model itself, that's handled by the model
	// undertaker.
	if err := s.st.DeleteModelArtifacts(ctx, job.EntityUUID); errors.Is(err, modelerrors.NotFound) {
		// The model has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("deleting model %q: %w", job.EntityUUID, err)
	}

	return nil
}
