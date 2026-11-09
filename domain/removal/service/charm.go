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

// CharmState describes charm removal operations.
type CharmState interface {
	// CharmExists reports whether the charm exists.
	CharmExists(ctx context.Context, charmUUID string) (bool, error)

	// CharmScheduleRemoval schedules a removal job for the charm with the
	// input UUID, to be actioned at the input time.
	CharmScheduleRemoval(ctx context.Context, removalUUID, charmUUID string, when time.Time) error

	// CharmHasApplicationReferences reports whether any application row still
	// references the charm, regardless of the application's life.
	CharmHasApplicationReferences(ctx context.Context, charmUUID string) (bool, error)

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

func (s *Service) scheduleCharmRemovals(ctx context.Context, charmUUIDs []string) error {
	for _, charmUUID := range charmUUIDs {
		if err := s.ScheduleCharmRemoval(ctx, corecharm.ID(charmUUID)); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// processCharmRemovalJob deletes a charm once it is no longer referenced by
// any application or unit, after removing any of its resources that are no
// longer referenced. Application references retain the charm but allow the job
// to complete. Unit references keep the job incomplete until their rows are
// deleted. Referenced resources also keep the job incomplete for retry.
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

	applicationReferenced, err := s.modelState.CharmHasApplicationReferences(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("checking application references for charm %q: %w", job.EntityUUID, err)
	}
	if applicationReferenced {
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
		s.logger.Debugf(ctx, "charm %q is pinned by referenced resources, will retry", job.EntityUUID)
		return errors.Errorf("charm %q is pinned by referenced resources", job.EntityUUID).
			Add(removalerrors.RemovalJobIncomplete)
	}

	if err := s.modelState.DeleteCharmIfUnused(ctx, job.EntityUUID); err != nil {
		return errors.Errorf("deleting charm %q: %w", job.EntityUUID, err)
	}
	return nil
}
