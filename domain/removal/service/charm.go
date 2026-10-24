// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// CharmState describes charm removal operations.
type CharmState interface {
	// CharmExists reports whether the charm exists.
	CharmExists(ctx context.Context, charmUUID string) (bool, error)

	// CharmHasReferencedResources reports whether any resource of the charm
	// is still referenced by an application, pending application, or unit.
	// Such resources pin the charm: its charm_resource rows cannot be
	// deleted until the references drop.
	CharmHasReferencedResources(ctx context.Context, charmUUID string) (bool, error)
}

// processCharmRemovalJob deletes a charm once it is no longer referenced by
// any application or unit, after removing any of its resources that are no
// longer referenced. If the charm is still in use, the job is simply
// deleted; the removal hooks schedule a new job when the last reference
// drops. If deletion is blocked by resources still referenced by an
// application, pending application, or unit, the job remains incomplete and
// is retried at the removal worker's polling interval.
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
		s.logger.Debugf(ctx, "charm %q is pinned by referenced resources, will retry", job.EntityUUID)
		return errors.Errorf("charm %q is pinned by referenced resources", job.EntityUUID).
			Add(removalerrors.RemovalJobIncomplete)
	}

	if err := s.modelState.DeleteCharmIfUnused(ctx, job.EntityUUID); err != nil {
		return errors.Errorf("deleting charm %q: %w", job.EntityUUID, err)
	}
	return nil
}
