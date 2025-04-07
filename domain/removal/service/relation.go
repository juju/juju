// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"time"

	"github.com/juju/juju/core/changestream"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
)

// RelationState describes retrieval and persistence
// methods specific to relation removal.
type RelationState interface {
	// RelationExists returns true if a relation exists with the input UUID.
	RelationExists(ctx context.Context, rUUID string) (bool, error)

	// RelationAdvanceLife ensures that there is no relation
	// identified by the input UUID, that is still alive.
	RelationAdvanceLife(ctx context.Context, rUUID string) error

	// RelationScheduleRemoval schedules a removal job for the relation with the
	// input UUID, qualified with the input force boolean.
	RelationScheduleRemoval(
		ctx context.Context, removalUUID, relUUID string, force bool, when time.Time,
	) error

	// NamespaceForWatchRemovals returns the table name whose UUIDs we
	// are watching in order to be notified of new removal jobs.
	NamespaceForWatchRemovals() string
}

// RemoveRelation checks if a relation with the input UUID exists.
// If it does, the relation is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
// The UUID for the scheduled removal job is returned.
// [relationerrors.RelationNotFound] is returned if no such relation exists.
func (s *Service) RemoveRelation(ctx context.Context, relUUID corerelation.UUID, force bool) (removal.UUID, error) {
	exists, err := s.st.RelationExists(ctx, relUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if relation %q exists: %w", relUUID, err)
	}
	if !exists {
		return "", errors.Errorf("relation %q does not exist", relUUID).Add(relationerrors.RelationNotFound)
	}

	if err := s.st.RelationAdvanceLife(ctx, relUUID.String()); err != nil {
		return "", errors.Errorf("relation %q: %w", relUUID, err)
	}

	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.st.RelationScheduleRemoval(
		ctx, jobUUID.String(), relUUID.String(), force, s.clock.Now().UTC(),
	); err != nil {
		return "", errors.Errorf("relation %q: %w", relUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for relation %q", jobUUID, relUUID)
	return jobUUID, nil
}

// WatchRemovals watches for scheduled removal jobs.
// The returned watcher emits the UUIDs of any inserted or updated jobs.
func (s *WatchableService) WatchRemovals() (watcher.StringsWatcher, error) {
	w, err := s.watcherFactory.NewUUIDsWatcher(s.st.NamespaceForWatchRemovals(), changestream.Changed)
	if err != nil {
		return nil, errors.Errorf("creating watcher for removals: %w", err)
	}
	return w, nil
}

func (s *Service) processRelationRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.RelationJob {
		return errors.Errorf("job type: %q not valid for relation removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	return nil
}
