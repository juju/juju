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
	"github.com/juju/juju/internal/errors"
)

// RelationState describes retrieval and persistence
// methods specific to relation removal.
type RelationState interface {
	// RelationExists returns true if a relation exists with the input UUID.
	RelationExists(ctx context.Context, rUUID string) (bool, error)

	// EnsureRelationNotAlive ensures that there is no relation
	// identified by the input UUID, that is still alive.
	EnsureRelationNotAlive(ctx context.Context, rUUID string) error

	// RelationScheduleRemoval schedules a removal job for the relation with the
	// input UUID, qualified with the input force boolean.
	RelationScheduleRemoval(ctx context.Context, removalUUID, relUUID string, force bool, when time.Time) error

	// NamespaceForWatchRemovals returns the table name whose UUIDs we
	// are watching in order to be notified of new removal jobs.
	NamespaceForWatchRemovals() string

	// GetRelationLife returns the life of the relation with the input UUID.
	GetRelationLife(ctx context.Context, rUUID string) (life.Life, error)

	// UnitNamesInScope returns the names of units in
	// the scope of the relation with the input UUID.
	UnitNamesInScope(ctx context.Context, rUUID string) ([]string, error)

	// DeleteRelationUnits deletes all relation unit records and their
	// associated settings for a relation. It effectively departs all
	// units from the scope of the input relation immediately.
	DeleteRelationUnits(ctx context.Context, rUUID string) error

	// DeleteRelation removes a relation from the database completely.
	DeleteRelation(ctx context.Context, rUUID string) error
}

// RemoveRelation checks if a relation with the input UUID exists.
// If it does, the relation is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// relation. This duration is ignored if the force argument is false.
// The UUID for the scheduled removal job is returned.
// [relationerrors.RelationNotFound] is returned if no such relation exists.
func (s *Service) RemoveRelation(
	ctx context.Context, relUUID corerelation.UUID, force bool, wait time.Duration) (_ removal.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	exists, err := s.st.RelationExists(ctx, relUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if relation %q exists: %w", relUUID, err)
	}
	if !exists {
		return "", errors.Errorf("relation %q does not exist", relUUID).Add(relationerrors.RelationNotFound)
	}

	if err := s.st.EnsureRelationNotAlive(ctx, relUUID.String()); err != nil {
		return "", errors.Errorf("relation %q: %w", relUUID, err)
	}

	var jUUID removal.UUID

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the relation if the normal destruction
			// workflows complete within the the wait duration.
			if _, err := s.relationScheduleRemoval(ctx, relUUID, false, 0); err != nil {
				return jUUID, errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal of relation %q", relUUID.String())
			wait = 0
		}
	}

	jUUID, err = s.relationScheduleRemoval(ctx, relUUID, force, wait)
	return jUUID, errors.Capture(err)
}

func (s *Service) relationScheduleRemoval(
	ctx context.Context, relUUID corerelation.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.st.RelationScheduleRemoval(
		ctx, jobUUID.String(), relUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("relation %q: %w", relUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for relation %q", jobUUID, relUUID)
	return jobUUID, nil
}

// processRelationRemovalJob deletes a relation if it is dying, and there are no
// units in scope.
// If force is true, the units are forcefully departed by deleting the
// relation_unit records before deleting the relation.
// Note that we do not need transactionality here:
//   - Life can only advance - it cannot become alive if dying or dead.
//   - Dying relations cannot be joined; we can delete settings and scopes
//     knowing that no new ones will be added.
//   - We can delete a relation with no unit participants, without fear of races.
//
// Note also, that relations don't actually ever transition to the dead state.
// They go from dying to gone. This is an artefact of behaviour under Mongo,
// preserved when relocating to Dqlite.
func (s *Service) processRelationRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.RelationJob {
		return errors.Errorf("job type: %q not valid for relation removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.st.GetRelationLife(ctx, job.EntityUUID)
	if err != nil {
		if errors.Is(err, relationerrors.RelationNotFound) {
			// The relation has already been removed.
			// Indicate success so that this job will be deleted.
			return nil
		}
		return errors.Errorf("getting relation %q life: %w", job.EntityUUID, err)
	}

	if l == life.Alive {
		return errors.Errorf("relation %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	inScope, err := s.st.UnitNamesInScope(ctx, job.EntityUUID)
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

		if err := s.st.DeleteRelationUnits(ctx, job.EntityUUID); err != nil {
			return errors.Errorf("departing units from relation %q scope: %w", job.EntityUUID, err)
		}
	}

	if err := s.st.DeleteRelation(ctx, job.EntityUUID); err != nil {
		return errors.Errorf("deleting relation %q: %w", job.EntityUUID, err)
	}
	return nil
}
