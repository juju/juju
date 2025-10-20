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
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
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
// [storageprovisioningerrors.StorageAttachmentNotFound] is returned if no such
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
		).Add(storageprovisioningerrors.StorageAttachmentNotFound)
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
	if errors.Is(err, storageprovisioningerrors.StorageAttachmentNotFound) {
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

// MarkFilesystemAttachmentAsDead marks the filesystem attachment as dead.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] if the filesystem
// attachment is not found.
// - [removalerrors.EntityStillAlive] if the filesystem attachment is alive.
func (s *Service) MarkFilesystemAttachmentAsDead(
	ctx context.Context, uuid storageprovisioning.FilesystemAttachmentUUID,
) error {
	return errors.New("not implemented: MarkFilesystemAttachmentAsDead")
}

// MarkVolumeAttachmentAsDead marks the volume attachment as dead.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] if the volume
// attachment is not found.
// - [removalerrors.EntityStillAlive] if the volume attachment is alive.
func (s *Service) MarkVolumeAttachmentAsDead(
	ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
) error {
	return errors.New("not implemented: MarkVolumeAttachmentAsDead")
}

// MarkVolumeAttachmentPlanAsDead marks the volume attachment plan as dead.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] if the volume
// attachment plan is not found.
// - [removalerrors.EntityStillAlive] if the volume attachment plan is alive.
func (s *Service) MarkVolumeAttachmentPlanAsDead(
	ctx context.Context, uuid storageprovisioning.VolumeAttachmentPlanUUID,
) error {
	return errors.New("not implemented: MarkVolumeAttachmentPlanAsDead")
}
