// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// ResourceState describes resource removal operations.
type ResourceState interface {
	// ResourceExists reports whether the resource exists.
	ResourceExists(ctx context.Context, resourceUUID string) (bool, error)

	// DeleteResourceIfUnused deletes a resource if it has no application,
	// pending application, or unit references. It reports whether the resource
	// was deleted or was already absent.
	DeleteResourceIfUnused(ctx context.Context, resourceUUID string) (bool, error)
}

func (s *Service) processResourceRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.ResourceJob {
		return errors.Errorf("job type: %q not valid for resource removal", job.RemovalType).
			Add(removalerrors.RemovalJobTypeNotValid)
	}

	exists, err := s.modelState.ResourceExists(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("checking if resource %q exists: %w", job.EntityUUID, err)
	}
	if !exists {
		return nil
	}

	removed, err := s.modelState.DeleteResourceIfUnused(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("deleting resource %q: %w", job.EntityUUID, err)
	}
	if !removed {
		return errors.Errorf("resource %q is still in use", job.EntityUUID).
			Add(removalerrors.RemovalJobIncomplete)
	}
	return nil
}
