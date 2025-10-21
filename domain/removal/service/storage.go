// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

// StorageState describes retrieval and persistence
// methods specific to storage removal.
type StorageState interface {
	// StorageAttachmentExists returns true if a storage attachment exists with
	// the input UUID.
	StorageAttachmentExists(ctx context.Context, saUUID string) (bool, error)

	// EnsureStorageAttachmentNotAlive ensures that there is no storage
	// attachment identified by the input UUID that is still alive.
	EnsureStorageAttachmentNotAlive(ctx context.Context, saUUID string) error

	// StorageAttachmentScheduleRemoval schedules a removal job for the storage
	// attachment with the input UUID, qualified with the input force boolean.
	StorageAttachmentScheduleRemoval(ctx context.Context, removalUUID, saUUID string, force bool, when time.Time) error

	// GetStorageAttachmentLife returns the life of the unit storage attachment
	// with the input UUID.
	GetStorageAttachmentLife(ctx context.Context, rUUID string) (life.Life, error)

	// DeleteStorageAttachment removes a unit storage attachment from the
	// database completely. If the unit attached to the storage was its owner,
	// then that record is deleted too.
	DeleteStorageAttachment(ctx context.Context, rUUID string) error
}

// RemoveStorageAttachment checks if a storage attachment with the input UUID
// exists.
// If it does, the attachment is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
//
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// attachment. This duration is ignored if the force argument is false.
// The UUID for the scheduled removal job is returned.
// [storageerrors.StorageAttachmentNotFound] is returned if no such
// relation exists.
func (s *Service) RemoveStorageAttachment(
	ctx context.Context, saUUID storageprovisioning.StorageAttachmentUUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.StorageAttachmentExists(ctx, saUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if storage attachment %q exists: %w", saUUID, err)
	}
	if !exists {
		return "", errors.Errorf(
			"storage attachment %q does not exist", saUUID,
		).Add(storageerrors.StorageAttachmentNotFound)
	}

	if err := s.modelState.EnsureStorageAttachmentNotAlive(ctx, saUUID.String()); err != nil {
		return "", errors.Errorf("relation %q: %w", saUUID, err)
	}

	var jUUID removal.UUID

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the attachment if the normal destruction
			// workflows complete within the wait duration.
			if _, err := s.storageAttachmentScheduleRemoval(ctx, saUUID, false, 0); err != nil {
				return jUUID, errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(
				ctx, "ignoring wait duration for non-forced removal of storage attachment %q", saUUID.String())
			wait = 0
		}
	}

	jUUID, err = s.storageAttachmentScheduleRemoval(ctx, saUUID, force, wait)
	return jUUID, errors.Capture(err)

}

// RemoveStorageAttachmentsFromAliveUnit is reponsible for removing one or more
// storage attachments from a unit that is still alive in the model. This
// operation can be considered a detatch of a storage instance from a unit.
//
// If the storage attachment exists and the unit it is attached to is alive the
// caller can expect that after this call the attachment is:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
//
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// attachment. This duration is ignored if the force argument is false.
//
// If removing the storage attachment would violate the minimum number of
// storage instances required by the unit's charm then this operation will fail.
// Regardless of if force is supplied this will not bypass fundemental checks.
// Force exists to force removal of the attachment resource and not the business
// logic that determines safety.
//
// The following errors may be returned:
// - [storageerrors.StorageAttachmentNotFound] if the supplied storage
// attachment uuid does not exist in the model.
// - [applicationerrors.UnitNotAlive] if the unit the storage attachment is
// conencted to is not alive.
func (s *Service) RemoveStorageAttachmentsFromAliveUnit(
	ctx context.Context,
	saUUID storageprovisioning.StorageAttachmentUUID,
	force bool,
	wait time.Duration,
) ([]removal.UUID, error) {
	return nil, errors.New("not implemented: coming soon")
}

func (s *Service) storageAttachmentScheduleRemoval(
	ctx context.Context, saUUID storageprovisioning.StorageAttachmentUUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.modelState.StorageAttachmentScheduleRemoval(
		ctx, jobUUID.String(), saUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("storage attachment %q: %w", saUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for storage attachment %q", jobUUID, saUUID)
	return jobUUID, nil
}

// processStorageAttachmentRemovalJob handles the fact that storage
// attachment sare endowed with the "life" characteristic.
// This endowment is historical, but perhaps needless - no action is
// really needed here except to delete the attachment.
// Associated removal workflows should have been triggered when
// we transitioned to "dying".
func (s *Service) processStorageAttachmentRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.StorageAttachmentJob {
		return errors.Errorf("job type: %q not valid for storage attachment removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetStorageAttachmentLife(ctx, job.EntityUUID)
	if errors.Is(err, storageerrors.StorageAttachmentNotFound) {
		// The storage attachment has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	}
	if err != nil {
		return errors.Errorf("getting storage attachment %q life: %w", job.EntityUUID, err)
	}

	if l == life.Alive {
		return errors.Errorf("storage attachment %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	if err := s.modelState.DeleteStorageAttachment(ctx, job.EntityUUID); err != nil {
		return errors.Errorf("deleting storage attachment %q: %w", job.EntityUUID, err)
	}

	return nil
}
