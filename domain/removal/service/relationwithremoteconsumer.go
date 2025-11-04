// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/internal/errors"
)

// RelationWithRemoteConsumer describes retrieval and persistence
// methods specific to remote relation removal.
type RelationWithRemoteConsumer interface {
	// RelationWithRemoteConsumerExists returns true if a relation exists with
	// the input UUID, and relates a synthetic application
	RelationWithRemoteConsumerExists(ctx context.Context, rUUID string) (bool, error)

	// EnsureRelationWithRemoteConsumerNotAliveCascade ensures that the relation
	// identified by the input UUID is not alive, and sets the synthetic units
	// in scope of this relation to dead.
	EnsureRelationWithRemoteConsumerNotAliveCascade(ctx context.Context, rUUID string) (internal.CascadedRelationWithRemoteConsumerLives, error)

	// RelationWithRemoteConsumerScheduleRemoval schedules a removal job for the
	// relation with the input UUID, qualified with the input force boolean.
	RelationWithRemoteConsumerScheduleRemoval(ctx context.Context, removalUUID, relUUID string, force bool, when time.Time) error

	// DeleteRelationWithRemoteConsumer deletes a remote relation record under
	// and all it's and anything dependent upon it. This includes synthetic
	// units.
	DeleteRelationWithRemoteConsumer(ctx context.Context, rUUID string) error
}

// RemoveRelationWithRemoteConsumer checks if a relation with the input UUID exists.
// If it does, the relation is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// remote application. This duration is ignored if the force argument is false.
// The UUID for the scheduled removal job is returned.
func (s *Service) RemoveRelationWithRemoteConsumer(
	ctx context.Context, relUUID corerelation.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.RelationWithRemoteConsumerExists(ctx, relUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if remote relation %q exists: %w", relUUID, err)
	}
	if !exists {
		return "", errors.Errorf("remote relation %q does not exist", relUUID).Add(relationerrors.RelationNotFound)
	}

	res, err := s.modelState.EnsureRelationWithRemoteConsumerNotAliveCascade(ctx, relUUID.String())
	if err != nil {
		return "", errors.Errorf("setting remote relation %q to dying: %w", relUUID, err)
	}

	var jUUID removal.UUID
	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the relation if the normal destruction
			// workflows complete within the wait duration.
			if _, err := s.relationWithRemoteConsumerScheduleRemoval(ctx, relUUID, false, 0); err != nil {
				return jUUID, errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal of remote relation %q", relUUID.String())
			wait = 0
		}
	}

	jUUID, err = s.relationWithRemoteConsumerScheduleRemoval(ctx, relUUID, force, wait)
	if err != nil {
		return "", errors.Errorf("scheduling removal job for remote relation %q: %w", relUUID, err)
	}

	// Depart the synthetic units here ourselves, since synthetic units don't
	// have their own uniter.
	for _, r := range res.SyntheticRelationUnitUUIDs {
		if err := s.modelState.LeaveScope(ctx, r); err != nil {
			return "", errors.Errorf("leaving scope for synthetic relation unit %q: %w", r, err)
		}
	}

	return jUUID, nil
}

func (s *Service) relationWithRemoteConsumerScheduleRemoval(
	ctx context.Context, relUUID corerelation.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.modelState.RelationWithRemoteConsumerScheduleRemoval(
		ctx, jobUUID.String(), relUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("scheduling remote relation %q for removal: %w", relUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for remote relation %q", jobUUID, relUUID)
	return jobUUID, nil
}

func (s *Service) processRelationWithRemoteConsumerRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.RelationWithRemoteConsumerJob {
		return errors.Errorf("job type: %q not valid for remote relation removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetRelationLife(ctx, job.EntityUUID)
	if errors.Is(err, relationerrors.RelationNotFound) {
		// The relation has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	}
	if err != nil {
		return errors.Errorf("getting remote relation %q life: %w", job.EntityUUID, err)
	}

	if l == life.Alive {
		return errors.Errorf("remote relation %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	inScope, err := s.modelState.UnitNamesInScope(ctx, job.EntityUUID)
	if err != nil {
		return errors.Capture(err)
	}

	if len(inScope) > 0 {
		// If this is a regular removal, we just exit and wait for
		// the job to be scheduled again for a later check.
		if !job.Force {
			s.logger.Infof(ctx, "removal job %q for relation %q is waiting for units to leave scope: %v",
				job.UUID, job.EntityUUID, inScope)

			return removalerrors.RemovalJobIncomplete
		}

		s.logger.Infof(ctx, "removal job %q for relation %q forcefully removing units from scope",
			job.UUID, job.EntityUUID)

		if err := s.modelState.DeleteRelationUnits(ctx, job.EntityUUID); err != nil {
			return errors.Errorf("departing units from relation %q scope: %w", job.EntityUUID, err)
		}
	}

	if err := s.modelState.DeleteRelationWithRemoteConsumer(ctx, job.EntityUUID); err != nil {
		return errors.Errorf("deleting remote relation %q: %w", job.EntityUUID, err)
	}

	return nil
}
