// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// rescheduleDelay is how far into the future a charm removal job is pushed
// when deletion is blocked by resources still pinned to an application.
const rescheduleDelay = 24 * time.Hour

// CharmState describes charm removal operations.
type CharmState interface {
	// CharmExists reports whether the charm exists.
	CharmExists(ctx context.Context, charmUUID string) (bool, error)

	// CharmScheduleRemoval schedules a removal job for the charm with the
	// input UUID, to be actioned at the input time.
	CharmScheduleRemoval(ctx context.Context, removalUUID, charmUUID string, when time.Time) error

	// CharmHasReferencedResources reports whether any resource of the charm
	// is still referenced by an application, pending application, or unit.
	// Such resources pin the charm: its charm_resource rows cannot be
	// deleted until the references drop.
	CharmHasReferencedResources(ctx context.Context, charmUUID string) (bool, error)
}

// ScheduleCharmRemoval schedules a removal job for the charm with the input
// UUID, to be actioned as soon as the removal worker picks it up.
func (s *Service) ScheduleCharmRemoval(ctx context.Context, charmUUID corecharm.ID) error {
	if err := charmUUID.Validate(); err != nil {
		return errors.Errorf("validating charm UUID: %w", err)
	}

	jobUUID, err := removal.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	if err := s.modelState.CharmScheduleRemoval(
		ctx, jobUUID.String(), charmUUID.String(), s.clock.Now().UTC(),
	); err != nil {
		return errors.Errorf("charm %q: %w", charmUUID, err)
	}
	s.logger.Infof(ctx, "scheduled removal job %q for charm %q", jobUUID, charmUUID)
	return nil
}

// processCharmRemovalJob deletes a charm once it is no longer referenced by
// any application or unit, after removing any of its resources that are no
// longer referenced. If the charm is still in use, the job is simply
// deleted; the removal hooks schedule a new job when the last reference
// drops. If deletion is blocked by resources still pinned to an application,
// the job is rescheduled instead of retried immediately.
func (s *Service) processCharmRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.CharmJob {
		return errors.Errorf("job type: %q not valid for charm removal", job.RemovalType).
			Add(removalerrors.RemovalJobTypeNotValid)
	}

	exists, err := s.modelState.CharmExists(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("checking if charm %q exists: %w", job.EntityUUID, err)
	}
	if !exists {
		return nil
	}

	if err := s.modelState.DeleteOrphanedResources(ctx, job.EntityUUID); err != nil {
		return errors.Errorf("deleting orphaned resources for charm %q: %w", job.EntityUUID, err)
	}

	pinned, err := s.modelState.CharmHasReferencedResources(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("checking resource references for charm %q: %w", job.EntityUUID, err)
	}
	if pinned {
		// A resource still referenced by an application, pending
		// application, or unit pins the charm (for example, resources scoped
		// to the previous charm after a refresh). Retry after the delay
		// rather than at every worker pass; the pin clears when the
		// reference drops.
		s.logger.Debugf(ctx, "charm %q pinned by referenced resources, rescheduling for %s",
			job.EntityUUID, rescheduleDelay)
		if err := s.modelState.RescheduleJob(
			ctx, job.UUID.String(), s.clock.Now().UTC().Add(rescheduleDelay),
		); err != nil {
			return errors.Errorf("rescheduling charm removal job %q: %w", job.UUID, err)
		}
		return errors.Errorf("charm %q is pinned by referenced resources", job.EntityUUID).
			Add(removalerrors.RemovalJobIncomplete)
	}

	if err := s.modelState.DeleteCharmIfUnused(ctx, job.EntityUUID); err != nil {
		return errors.Errorf("deleting charm %q: %w", job.EntityUUID, err)
	}
	return nil
}
