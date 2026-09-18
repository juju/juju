// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

const (
	// charmOrphanGracePeriod is the minimum age of an unreferenced charm
	// before the periodic scan schedules it for removal. It guards against
	// charms that are mid-deploy: a charm upload becomes available before
	// the application row that references it exists.
	charmOrphanGracePeriod = time.Hour

	// charmRemovalRescheduleDelay is how far into the future a charm
	// removal job is rescheduled when charm deletion is blocked by a
	// foreign key constraint, such as resources still referenced by a
	// refreshed application.
	charmRemovalRescheduleDelay = 24 * time.Hour
)

// CharmState describes retrieval and persistence methods specific to charm
// removal.
type CharmState interface {
	// CharmScheduleRemoval schedules a removal job for the charm with the
	// input UUID, to be executed at or after the input time.
	CharmScheduleRemoval(ctx context.Context, removalUUID, charmUUID string, when time.Time) error

	// CharmExists returns true if a charm exists with the input UUID.
	CharmExists(ctx context.Context, charmUUID string) (bool, error)

	// RescheduleJob updates the scheduled time of the removal job with the
	// input UUID.
	RescheduleJob(ctx context.Context, jobUUID string, when time.Time) error

	// GetUnusedCharmUUIDs returns the UUIDs of charms that are available,
	// were created before the input time, and are not referenced by any
	// application or unit.
	GetUnusedCharmUUIDs(ctx context.Context, olderThan time.Time) ([]string, error)
}

// ScheduleCharmRemoval schedules a removal job for the charm with the input
// UUID, for immediate execution. If the charm is still referenced when the
// job executes, the job completes without effect; the hooks that observe
// the charm's last reference being dropped schedule a new job.
func (s *Service) ScheduleCharmRemoval(ctx context.Context, charmUUID string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	jobUUID, err := removal.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}

	if err := s.modelState.CharmScheduleRemoval(
		ctx, jobUUID.String(), charmUUID, s.clock.Now().UTC(),
	); err != nil {
		return errors.Errorf("charm %q: %w", charmUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for charm %q", jobUUID, charmUUID)
	return nil
}

// ScheduleCharmRemovalsForUnusedCharms schedules removal jobs for all charms
// that are not referenced by any application or unit and are older than the
// orphan grace period. Charms that already have a pending charm removal job
// are skipped.
func (s *Service) ScheduleCharmRemovalsForUnusedCharms(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	cutoff := s.clock.Now().UTC().Add(-charmOrphanGracePeriod)
	charmUUIDs, err := s.modelState.GetUnusedCharmUUIDs(ctx, cutoff)
	if err != nil {
		return errors.Errorf("getting unused charms: %w", err)
	}
	if len(charmUUIDs) == 0 {
		return nil
	}

	jobs, err := s.modelState.GetAllJobs(ctx)
	if err != nil {
		return errors.Errorf("getting removal jobs: %w", err)
	}
	pending := set.NewStrings()
	for _, job := range jobs {
		if job.RemovalType == removal.CharmJob {
			pending.Add(job.EntityUUID)
		}
	}

	for _, charmUUID := range charmUUIDs {
		if pending.Contains(charmUUID) {
			continue
		}
		if err := s.ScheduleCharmRemoval(ctx, charmUUID); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// processCharmRemovalJob deletes a charm that is no longer referenced by any
// application or unit.
func (s *Service) processCharmRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.CharmJob {
		return errors.Errorf("job type: %q not valid for charm removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	charmUUID := job.EntityUUID

	exists, err := s.modelState.CharmExists(ctx, charmUUID)
	if err != nil {
		return errors.Errorf("checking charm %q exists: %w", charmUUID, err)
	}
	if !exists {
		// The charm has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	}

	// Orphaned resources pin the charm via foreign key; delete them first.
	if err := s.modelState.DeleteOrphanedResources(ctx, charmUUID); err != nil {
		return errors.Errorf("deleting orphaned resources for charm %q: %w", charmUUID, err)
	}

	if err := s.modelState.DeleteCharmIfUnused(ctx, charmUUID); err != nil {
		if internaldatabase.IsErrConstraintForeignKey(err) {
			// The charm is pinned by rows that cannot be deleted yet, such
			// as resources still referenced by an application. Reschedule
			// the job instead of deleting it, so the charm is revisited.
			when := s.clock.Now().UTC().Add(charmRemovalRescheduleDelay)
			if rErr := s.modelState.RescheduleJob(ctx, job.UUID.String(), when); rErr != nil {
				return errors.Errorf("rescheduling charm removal job %q: %w", job.UUID, rErr)
			}
			return errors.Errorf("charm %q is pinned by foreign key references", charmUUID).
				Add(removalerrors.RemovalJobIncomplete)
		}
		return errors.Errorf("deleting charm %q: %w", charmUUID, err)
	}

	// DeleteCharmIfUnused also succeeds without deleting when the charm is
	// still referenced by an application or unit. Either way the job is
	// done: if the charm remains, the hooks that observe its last reference
	// being dropped will schedule a new job.
	return nil
}
