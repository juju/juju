// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
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

	// EnsureStorageAttachmentNotAliveWithFulfilment ensures that there is no
	// storage attachment identified by the input UUID that is still alive
	// after this call. This condition is only realised when the storage
	// fulfilment for the units charm is met by the removal.
	//
	// Fulfilment expectation exists to assert the state of the world for which
	// the ensure operation was computed on top of.
	//
	// The following errors may be returned:
	// - [removalerrors.StorageFulfilmentNotMet] when the fulfilment requiremnt
	// fails.
	EnsureStorageAttachmentNotAliveWithFulfilment(
		ctx context.Context,
		saUUID string,
		fulfilment int,
	) error

	// StorageAttachmentScheduleRemoval schedules a removal job for the storage
	// attachment with the input UUID, qualified with the input force boolean.
	StorageAttachmentScheduleRemoval(ctx context.Context, removalUUID, saUUID string, force bool, when time.Time) error

	// GetDetachInfoForStorageAttachment returns the information required to
	// compute what a units storage requirement will look like after having
	// removed the storage attachment.
	//
	// This information can be used to establish if detaching storage from the
	// unit would violate the expectations of the unit's charm.
	//
	// The following errors may be returned:
	// - [storageerrors.StorageAttachmentNotFound] if the storage attachment
	// no longer exists in the model.
	GetDetachInfoForStorageAttachment(
		context.Context, string,
	) (internal.StorageAttachmentDetachInfo, error)

	// GetStorageAttachmentLife returns the life of the unit storage attachment
	// with the input UUID.
	GetStorageAttachmentLife(ctx context.Context, rUUID string) (life.Life, error)

	// EnsureStorageAttachmentDeadCascade ensures that the storage attachment is
	// dead and that all filesystem attachments, volume attachments and volume
	// attachment plans are dying.
	EnsureStorageAttachmentDeadCascade(
		ctx context.Context, uuid string,
	) (internal.CascadedStorageProvisionedAttachmentLives, error)

	// DeleteStorageAttachment removes a unit storage attachment from the
	// database completely. If the unit attached to the storage was its owner,
	// then that record is deleted too.
	DeleteStorageAttachment(ctx context.Context, rUUID string) error

	// EnsureStorageInstanceNotAliveCascade ensures that there is no storage
	// instance identified by the input UUID, that is still alive.
	EnsureStorageInstanceNotAliveCascade(
		ctx context.Context, siUUID string, obliterate bool,
	) (internal.CascadedStorageFilesystemVolumeLives, error)

	// GetStorageInstanceLife returns the life of the storage instance with
	// the input UUID.
	GetStorageInstanceLife(
		ctx context.Context, siUUID string,
	) (life.Life, error)

	// StorageInstancecheduleRemoval schedules a removal job for the storage
	// instance with the input UUID, qualified with the input force boolean.
	StorageInstanceScheduleRemoval(
		ctx context.Context,
		removalUUID, siUUID string,
		force bool, when time.Time,
	) error

	// CheckStorageInstanceHasNoChildren returns true if the storage instance
	// with the input UUID has no child filesystem or volume.
	CheckStorageInstanceHasNoChildren(
		ctx context.Context, siUUID string,
	) (bool, error)

	// DeleteStorageInstance removes a storage instance from the database
	// completely.
	DeleteStorageInstance(ctx context.Context, siUUID string) error

	// GetVolumeLife returns the life of the volume with the input UUID.
	GetVolumeLife(ctx context.Context, volUUID string) (life.Life, error)

	// GetVolumeStatus returns the status of the volume indicated by the input
	// UUID.
	GetVolumeStatus(ctx context.Context, volUUID string) (int, error)

	// SetVolumeStatus changes the status of the volume indicated by the input
	// UUID and status value.
	SetVolumeStatus(ctx context.Context, volUUID string, status int) error

	// VolumeScheduleRemoval schedules a removal job for the volume with the
	// input UUID, qualified with the input force boolean.
	VolumeScheduleRemoval(ctx context.Context, removalUUID, volUUID string, force bool, when time.Time) error

	// DeleteVolume removes the volume with the input UUID.
	DeleteVolume(ctx context.Context, volUUID string) error

	// GetFilesystemLife returns the life of the filesystem with the input UUID.
	GetFilesystemLife(ctx context.Context, fsUUID string) (life.Life, error)

	// GetFilesystemStatus returns the status of the filesystem indicated by the
	// input UUID.
	GetFilesystemStatus(ctx context.Context, fsUUID string) (int, error)

	// SetFilesystemStatus changes the status of the filesystem indicated by the
	// input UUID and status value.
	SetFilesystemStatus(ctx context.Context, fsUUID string, status int) error

	// CheckVolumeBackedFilesystemCrossProvisioned returns true if the specified
	// uuid is a filesystem that is volume backed, where the filesystem is not
	// owned by a machine, where the filesystem is machine provisioned and where
	// the volume is model provisioned. This is to handle filesystems that will
	// never be de-provisioned by a provisioner.
	CheckVolumeBackedFilesystemCrossProvisioned(
		ctx context.Context, fsUUID string,
	) (bool, error)

	// DeleteFilesystem removes the filesystem with the input UUID.
	DeleteFilesystem(ctx context.Context, fsUUID string) error

	// FilesystemScheduleRemoval schedules a removal job for the filesystem with
	// the input UUID, qualified with the input force boolean.
	FilesystemScheduleRemoval(ctx context.Context, removalUUID, fsUUID string, force bool, when time.Time) error

	// GetFilesystemAttachmentLife returns the life of the filesystem attachment
	// indicated by the supplied UUID.
	GetFilesystemAttachmentLife(
		ctx context.Context, rUUID string,
	) (life.Life, error)

	// MarkFilesystemAttachmentAsDead updates the life to dead of the filesystem
	// attachment indicated by the supplied UUID.
	MarkFilesystemAttachmentAsDead(ctx context.Context, rUUID string) error

	// DeleteFilesystemAttachment removes the filesystem attachment with the
	// input UUID.
	DeleteFilesystemAttachment(ctx context.Context, fsaUUID string) error

	// FilesystemAttachmentScheduleRemoval schedules a removal job for the
	// filesystem attachment with the input UUID, qualified with the input force
	// boolean.
	FilesystemAttachmentScheduleRemoval(
		ctx context.Context,
		removalUUID, fsaUUID string,
		force bool, when time.Time,
	) error

	// GetVolumeAttachmentLife returns the life of the volume attachment
	// indicated by the supplied UUID.
	GetVolumeAttachmentLife(
		ctx context.Context, rUUID string,
	) (life.Life, error)

	// MarkVolumeAttachmentAsDead updates the life to dead of the volume
	// attachment indicated by the supplied UUID.
	MarkVolumeAttachmentAsDead(ctx context.Context, rUUID string) error

	// DeleteVolumeAttachment removes the volume attachment with the input UUID.
	DeleteVolumeAttachment(ctx context.Context, vaUUID string) error

	// VolumeAttachmentScheduleRemoval schedules a removal job for the volume
	// attachment with the input UUID, qualified with the input force boolean.
	VolumeAttachmentScheduleRemoval(
		ctx context.Context,
		removalUUID, vaUUID string,
		force bool, when time.Time,
	) error

	// GetVolumeAttachmentPlanLife returns the life of the volume attachment
	// plan indicated by the supplied UUID.
	GetVolumeAttachmentPlanLife(
		ctx context.Context, rUUID string,
	) (life.Life, error)

	// MarkVolumeAttachmentPlanAsDead updates the life to dead of the volume
	// attachment plan indicated by the supplied UUID.
	MarkVolumeAttachmentPlanAsDead(ctx context.Context, rUUID string) error

	// DeleteVolumeAttachmentPlan removes the volume attachment plan with the
	// input UUID.
	DeleteVolumeAttachmentPlan(ctx context.Context, vapUUID string) error

	// VolumeAttachmentPlanScheduleRemoval schedules a removal job for the
	// volume attachment plan with the input UUID, qualified with the input
	// force boolean.
	VolumeAttachmentPlanScheduleRemoval(
		ctx context.Context,
		removalUUID, vaUUID string,
		force bool, when time.Time,
	) error
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

// RemoveStorageAttachmentFromAliveUnit is responsible for removing a storage
// attachment from a unit that is still alive in the model. This operation can
// be considered a detatch of a storage instance from a unit.
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
// Regardless of if force is supplied this will not bypass fundamental checks.
// Force exists to force removal of the attachment resource and not the business
// logic that determines safety.
//
// The following errors may be returned:
// - [storageerrors.StorageAttachmentNotFound] if the supplied storage
// attachment uuid does not exist in the model.
// - [applicationerrors.UnitNotAlive] if the unit the storage attachment is
// conencted to is not alive.
// [applicationerrors.UnitStorageMinViolation] if removing a storage
// attachment would violate the charm minimums required for the unit.
func (s *Service) RemoveStorageAttachmentFromAliveUnit(
	ctx context.Context,
	saUUID storageprovisioning.StorageAttachmentUUID,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	detachInfo, err := s.modelState.GetDetachInfoForStorageAttachment(
		ctx, saUUID.String(),
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Declare value here before goto.
	proposedNewFulfilment := detachInfo.CountFulfilment - 1

	// If the storage attachment is already dead we can get on with the next
	// item.
	if life.Life(detachInfo.Life) != life.Alive {
		goto RemovalJob
	}

	// Force does not allow the caller to ride through this check. Force
	// is about forcibly removing the storage attachment.
	if life.Life(detachInfo.UnitLife) != life.Alive {
		return "", errors.Errorf(
			"storage attachment %q cannot be removed because its unit %q is not alive",
			saUUID, detachInfo.UnitUUID,
		).Add(applicationerrors.UnitNotAlive)
	}

	if proposedNewFulfilment < detachInfo.RequiredCountMin {
		return "", errors.Errorf(
			"removing storage attachment %q would violate the minimum number %d of %q storage instances required by unit %q",
			saUUID,
			detachInfo.RequiredCountMin,
			detachInfo.CharmStorageName,
			detachInfo.UnitUUID,
		).Add(applicationerrors.UnitStorageMinViolation{
			CharmStorageName: detachInfo.CharmStorageName,
			RequiredMinimum:  detachInfo.RequiredCountMin,
			UnitUUID:         detachInfo.UnitUUID,
		})
	}

	err = s.modelState.EnsureStorageAttachmentNotAliveWithFulfilment(
		ctx, saUUID.String(), proposedNewFulfilment,
	)
	if errors.Is(err, removalerrors.StorageFulfilmentNotMet) {
		return "", errors.Errorf(
			"removing storage attachment %q would violate the minimum number %d of %q storage instances required by unit %q",
			saUUID,
			detachInfo.RequiredCountMin,
			detachInfo.CharmStorageName,
			detachInfo.UnitUUID,
		).Add(applicationerrors.UnitStorageMinViolation{
			CharmStorageName: detachInfo.CharmStorageName,
			RequiredMinimum:  detachInfo.RequiredCountMin,
			UnitUUID:         detachInfo.UnitUUID,
		})
	} else if err != nil {
		return "", errors.Capture(err)
	}

RemovalJob:
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
	if err != nil {
		return "", errors.Capture(err)
	}
	return jUUID, nil
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
// attachments are endowed with the "life" characteristic.
// This endowment is historical, but perhaps needless - no action is
// really needed here except to delete the attachment.
// Associated removal workflows should have been triggered when
// we transitioned to "dying".
func (s *Service) processStorageAttachmentRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.StorageAttachmentJob {
		return errors.Errorf(
			"job type: %q not valid for storage attachment removal",
			job.RemovalType,
		).Add(removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetStorageAttachmentLife(ctx, job.EntityUUID)
	if errors.Is(err, storageerrors.StorageAttachmentNotFound) {
		// The storage attachment has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	}
	if err != nil {
		return errors.Errorf(
			"getting storage attachment %q life: %w", job.EntityUUID, err,
		)
	}

	if l == life.Alive {
		return errors.Errorf(
			"storage attachment %q is alive", job.EntityUUID,
		).Add(removalerrors.EntityStillAlive)
	} else if !job.Force && l == life.Dying {
		return errors.Errorf(
			"storage attachment %q is not dead", job.EntityUUID,
		).Add(removalerrors.EntityNotDead)
	}

	if job.Force && l == life.Dying {
		cascade, err := s.modelState.EnsureStorageAttachmentDeadCascade(
			ctx, job.EntityUUID)
		if errors.Is(err, storageerrors.StorageAttachmentNotFound) {
			return errors.Errorf(
				"storage attachment %q not found", job.EntityUUID,
			).Add(storageerrors.StorageAttachmentNotFound)
		} else if err != nil {
			return errors.Errorf(
				"ensuring storage attachment %q is dead: %w",
				job.EntityUUID, err,
			)
		}

		// NOTE: filesystem attachments, volume attachments and volume attachment
		// plans have their removal jobs scheduled when the storage attachment goes
		// to dying. But due to the entities not also going to dying, it is
		// important that we schedule these here too, for completeness.

		for _, fsaUUID := range cascade.FileSystemAttachmentUUIDs {
			uuid := storageprovisioning.FilesystemAttachmentUUID(fsaUUID)
			_, err := s.filesystemAttachmentScheduleRemoval(ctx, uuid, false, 0)
			if err != nil {
				return errors.Errorf(
					"scheduling filesystem attachment %q removal job: %w",
					uuid, err,
				)
			}
		}

		for _, vaUUID := range cascade.VolumeAttachmentUUIDs {
			uuid := storageprovisioning.VolumeAttachmentUUID(vaUUID)
			_, err := s.volumeAttachmentScheduleRemoval(ctx, uuid, false, 0)
			if err != nil {
				return errors.Errorf(
					"scheduling volume attachment %q removal job: %w",
					uuid, err,
				)
			}
		}

		for _, vapUUID := range cascade.VolumeAttachmentPlanUUIDs {
			uuid := storageprovisioning.VolumeAttachmentPlanUUID(vapUUID)
			_, err := s.volumeAttachmentPlanScheduleRemoval(ctx, uuid, false, 0)
			if err != nil {
				return errors.Errorf(
					"scheduling volume attachment plan %q removal job: %w",
					uuid, err,
				)
			}
		}
	}

	if err := s.modelState.DeleteStorageAttachment(ctx, job.EntityUUID); err != nil {
		return errors.Errorf("deleting storage attachment %q: %w", job.EntityUUID, err)
	}

	return nil
}

// MarkStorageAttachmentAsDead marks the storage attachment as dead and cascade
// removes the filesystem attachments, volume attachments and volume attachment
// plans.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied storage attachment UUID is not
// valid.
// - [storageerrors.StorageAttachmentNotFound] if the storage attachmemt is not
// found.
// - [removalerrors.EntityStillAlive] if the storage attachment is alive.
func (s *Service) MarkStorageAttachmentAsDead(
	ctx context.Context, uuid storageprovisioning.StorageAttachmentUUID,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := uuid.Validate()
	if err != nil {
		return errors.Errorf(
			"validating storage attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	l, err := s.modelState.GetStorageAttachmentLife(ctx, uuid.String())
	if errors.Is(err, storageerrors.StorageAttachmentNotFound) {
		return errors.Errorf(
			"storage attachment %q not found", uuid,
		).Add(storageerrors.StorageAttachmentNotFound)
	} else if err != nil {
		return errors.Errorf(
			"getting storage attachment %q life: %w", uuid, err,
		)
	}

	if l == life.Alive {
		return errors.Errorf(
			"storage attachment %q is alive", uuid,
		).Add(removalerrors.EntityStillAlive)
	}

	cascade, err := s.modelState.EnsureStorageAttachmentDeadCascade(
		ctx, uuid.String())
	if errors.Is(err, storageerrors.StorageAttachmentNotFound) {
		return errors.Errorf(
			"storage attachment %q not found", uuid,
		).Add(storageerrors.StorageAttachmentNotFound)
	} else if err != nil {
		return errors.Errorf(
			"ensuring storage attachment %q is dead: %w", uuid, err,
		)
	}

	// NOTE: filesystem attachments, volume attachments and volume attachment
	// plans have their removal jobs scheduled when the storage attachment goes
	// to dying. But due to the entities not also going to dying, it is
	// important that we schedule these here too, for completeness.

	for _, fsaUUID := range cascade.FileSystemAttachmentUUIDs {
		uuid := storageprovisioning.FilesystemAttachmentUUID(fsaUUID)
		_, err := s.filesystemAttachmentScheduleRemoval(ctx, uuid, false, 0)
		if err != nil {
			return errors.Errorf(
				"scheduling filesystem attachment %q removal job: %w",
				uuid, err,
			)
		}
	}

	for _, vaUUID := range cascade.VolumeAttachmentUUIDs {
		uuid := storageprovisioning.VolumeAttachmentUUID(vaUUID)
		_, err := s.volumeAttachmentScheduleRemoval(ctx, uuid, false, 0)
		if err != nil {
			return errors.Errorf(
				"scheduling volume attachment %q removal job: %w",
				uuid, err,
			)
		}
	}

	for _, vapUUID := range cascade.VolumeAttachmentPlanUUIDs {
		uuid := storageprovisioning.VolumeAttachmentPlanUUID(vapUUID)
		_, err := s.volumeAttachmentPlanScheduleRemoval(ctx, uuid, false, 0)
		if err != nil {
			return errors.Errorf(
				"scheduling volume attachment plan %q removal job: %w",
				uuid, err,
			)
		}
	}

	return nil
}

// RemoveStorageInstance ensures that the specified storage instance is no
// longer alive, scheduling removal jobs if needed and if specified, mark the
// volume and filesystems for obliteration.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied storage instance UUID is not valid.
// - [storageerrors.StorageInstanceNotFound] if the storage instance is not
// found.
func (s *Service) RemoveStorageInstance(
	ctx context.Context,
	uuid storage.StorageInstanceUUID,
	force bool, wait time.Duration,
	obliterate bool,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := uuid.Validate()
	if err != nil {
		return errors.Errorf(
			"validating storage instance uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	cascaded, err := s.modelState.EnsureStorageInstanceNotAliveCascade(
		ctx, uuid.String(), obliterate)
	if errors.Is(err, storageerrors.StorageInstanceNotFound) {
		return errors.Errorf(
			"storage instance %q not found", uuid,
		).Add(storageerrors.StorageInstanceNotFound)
	} else if err != nil {
		return errors.Errorf(
			"ensuring storage instance %q is not alive: %w", uuid, err,
		)
	}

	if force && wait > 0 {
		if _, err := s.storageInstanceScheduleRemoval(
			ctx, uuid, false, 0,
		); err != nil {
			return errors.Capture(err)
		}
	}
	if _, err := s.storageInstanceScheduleRemoval(
		ctx, uuid, force, wait,
	); err != nil {
		return errors.Capture(err)
	}

	if cascaded.FileSystemUUID != nil {
		fsUUID := storageprovisioning.FilesystemUUID(*cascaded.FileSystemUUID)
		if force && wait > 0 {
			if _, err := s.filesystemScheduleRemoval(
				ctx, fsUUID, false, 0,
			); err != nil {
				return errors.Capture(err)
			}
		}
		if _, err := s.filesystemScheduleRemoval(
			ctx, fsUUID, force, wait,
		); err != nil {
			return errors.Capture(err)
		}
	}

	if cascaded.VolumeUUID != nil {
		volUUID := storageprovisioning.VolumeUUID(*cascaded.VolumeUUID)
		if force && wait > 0 {
			if _, err := s.volumeScheduleRemoval(
				ctx, volUUID, false, 0,
			); err != nil {
				return errors.Capture(err)
			}
		}
		if _, err := s.volumeScheduleRemoval(
			ctx, volUUID, force, wait,
		); err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

func (s *Service) storageInstanceScheduleRemoval(
	ctx context.Context,
	siUUID storage.StorageInstanceUUID,
	force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	err = s.modelState.StorageInstanceScheduleRemoval(
		ctx, jobUUID.String(), siUUID.String(),
		force, s.clock.Now().UTC().Add(wait),
	)
	if err != nil {
		return "", errors.Errorf("storage instance %q: %w", siUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for storage instance %q",
		jobUUID, siUUID)
	return jobUUID, nil
}

// processStorageInstanceRemovalJob handles the deletion of a storage instance.
func (s *Service) processStorageInstanceRemovalJob(
	ctx context.Context, job removal.Job,
) error {
	if job.RemovalType != removal.StorageInstanceJob {
		return errors.Errorf(
			"job type: %q not valid for storage instance removal",
			job.RemovalType,
		).Add(removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetStorageInstanceLife(ctx, job.EntityUUID)
	if errors.Is(err, storageerrors.StorageInstanceNotFound) {
		// The storage instance has already been removed.
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting storage instance %q life: %w", job.EntityUUID, err,
		)
	}

	if l == life.Alive {
		return errors.Errorf(
			"storage instance %q is alive", job.EntityUUID,
		).Add(removalerrors.EntityStillAlive)
	}

	if !job.Force {
		canRemove, err := s.modelState.CheckStorageInstanceHasNoChildren(
			ctx, job.EntityUUID)
		if err != nil {
			return errors.Errorf(
				"checking storage instance %q has no children: %w",
				job.EntityUUID, err,
			)
		}
		if !canRemove {
			return errors.Errorf(
				"storage instance %q still has children", job.EntityUUID,
			).Add(removalerrors.StorageInstanceHasChildren)
		}
	}

	err = s.modelState.DeleteStorageInstance(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf(
			"deleting storage instance %q: %w", job.EntityUUID, err,
		)
	}

	return nil
}

// MarkFilesystemAttachmentAsDead marks the filesystem attachment as dead.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied filesystem attachment UUID is not
// valid.
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] if the filesystem
// attachment is not found.
// - [removalerrors.EntityStillAlive] if the filesystem attachment is alive.
func (s *Service) MarkFilesystemAttachmentAsDead(
	ctx context.Context, uuid storageprovisioning.FilesystemAttachmentUUID,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := uuid.Validate()
	if err != nil {
		return errors.Errorf(
			"validating filesystem attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	l, err := s.modelState.GetFilesystemAttachmentLife(ctx, uuid.String())
	if err != nil {
		return errors.Errorf(
			"getting filesystem attachment %q life: %w", uuid, err,
		)
	}
	if l == life.Alive {
		return errors.Errorf(
			"filesystem attachment %q is alive", uuid,
		).Add(removalerrors.EntityStillAlive)
	} else if l == life.Dead {
		return nil
	}

	err = s.modelState.MarkFilesystemAttachmentAsDead(ctx, uuid.String())
	if err != nil {
		return errors.Errorf(
			"marking filesystem attachment %q as dead: %w", uuid, err,
		)
	}

	return nil
}

// MarkVolumeAttachmentAsDead marks the volume attachment as dead.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied volume attachment UUID is not
// valid.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] if the volume
// attachment is not found.
// - [removalerrors.EntityStillAlive] if the volume attachment is alive.
func (s *Service) MarkVolumeAttachmentAsDead(
	ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := uuid.Validate()
	if err != nil {
		return errors.Errorf(
			"validating volume attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	l, err := s.modelState.GetVolumeAttachmentLife(ctx, uuid.String())
	if err != nil {
		return errors.Errorf(
			"getting volume attachment %q life: %w", uuid, err,
		)
	}
	if l == life.Alive {
		return errors.Errorf(
			"volume attachment %q is alive", uuid,
		).Add(removalerrors.EntityStillAlive)
	} else if l == life.Dead {
		return nil
	}

	err = s.modelState.MarkVolumeAttachmentAsDead(ctx, uuid.String())
	if err != nil {
		return errors.Errorf(
			"marking volume attachment %q as dead: %w", uuid, err,
		)
	}

	return nil
}

// MarkVolumeAttachmentPlanAsDead marks the volume attachment plan as dead.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied volume attachment plan UUID is not
// valid.
// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] if the volume
// attachment plan is not found.
// - [removalerrors.EntityStillAlive] if the volume attachment plan is alive.
func (s *Service) MarkVolumeAttachmentPlanAsDead(
	ctx context.Context, uuid storageprovisioning.VolumeAttachmentPlanUUID,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := uuid.Validate()
	if err != nil {
		return errors.Errorf(
			"validating volume attachment plan uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	l, err := s.modelState.GetVolumeAttachmentPlanLife(ctx, uuid.String())
	if err != nil {
		return errors.Errorf(
			"getting volume attachment plan %q life: %w", uuid, err,
		)
	}
	if l == life.Alive {
		return errors.Errorf(
			"volume attachment plan %q is alive", uuid,
		).Add(removalerrors.EntityStillAlive)
	} else if l == life.Dead {
		return nil
	}

	err = s.modelState.MarkVolumeAttachmentPlanAsDead(ctx, uuid.String())
	if err != nil {
		return errors.Errorf(
			"marking volume attachment plan %q as dead: %w", uuid, err,
		)
	}

	return nil
}

// RemoveDeadFilesystem is to be called from the storage provisoner to
// finally remove a dead filesystem that it has been gracefully cleaned up.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied filesystem UUID is not valid.
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the uuid.
// - [storageprovisioningerrors.FilesystemNotDead] when the filesystem was found
// but is either alive or dying, when it is expected to be dead.
func (s *Service) RemoveDeadFilesystem(
	ctx context.Context, uuid storageprovisioning.FilesystemUUID,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := uuid.Validate()
	if err != nil {
		return errors.Errorf(
			"validating filesystem uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	fsLife, err := s.modelState.GetFilesystemLife(ctx, uuid.String())
	if err != nil {
		return errors.Errorf("getting filesystem life for %q: %w", uuid, err)
	}
	if fsLife != life.Dead {
		return errors.Errorf(
			"filesystem %q is not dead", uuid,
		).Add(storageprovisioningerrors.FilesystemNotDead)
	}

	tombstone := int(status.StorageFilesystemStatusTypeTombstone)
	err = s.modelState.SetFilesystemStatus(ctx, uuid.String(), tombstone)
	if err != nil {
		return errors.Errorf(
			"setting filesystem tombstone status for %q: %w", uuid, err,
		)
	}

	return nil
}

func (s *Service) filesystemScheduleRemoval(
	ctx context.Context,
	fsUUID storageprovisioning.FilesystemUUID,
	force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	err = s.modelState.FilesystemScheduleRemoval(
		ctx, jobUUID.String(), fsUUID.String(),
		force, s.clock.Now().UTC().Add(wait),
	)
	if err != nil {
		return "", errors.Errorf("filesystem %q: %w", fsUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for filesystem %q", jobUUID,
		fsUUID)
	return jobUUID, nil
}

// processStorageFilesystemRemovalJob handles the deletion of the filesystem.
// For a non-forced removal job, the filesystem is deleted once the life is dead
// and the status is tombstone. For forced removal job, deletion of a filesystem
// happens once the filesystem is no longer alive.
func (s *Service) processStorageFilesystemRemovalJob(
	ctx context.Context, job removal.Job,
) error {
	if job.RemovalType != removal.StorageFilesystemJob {
		return errors.Errorf(
			"job type: %q not valid for storage filesystem removal",
			job.RemovalType,
		).Add(removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetFilesystemLife(ctx, job.EntityUUID)
	if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		// The filesystem has already been removed.
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting filesystem %q life: %w", job.EntityUUID, err,
		)
	}

	if l == life.Alive {
		return errors.Errorf(
			"filesystem %q is alive", job.EntityUUID,
		).Add(removalerrors.EntityStillAlive)
	} else if !job.Force && l == life.Dying {
		return errors.Errorf(
			"filesystem %q is not dead", job.EntityUUID,
		).Add(removalerrors.EntityNotDead)
	} else if !job.Force {
		sv, err := s.modelState.GetFilesystemStatus(ctx, job.EntityUUID)
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			// The filesystem has already been removed.
			return nil
		} else if err != nil {
			return errors.Errorf(
				"getting filesystem %q status: %w", job.EntityUUID, err,
			)
		}
		if sv == int(status.StorageFilesystemStatusTypeTombstone) {
			goto deleteFilesystem
		}
		// A filesystem that is machine provisioned, but backed by a volume that
		// is model provisioned, it is impossible for it to reach the tombstone
		// status, since there is no storage provisioner responsible for it.
		canRemove, err := s.modelState.CheckVolumeBackedFilesystemCrossProvisioned(
			ctx, job.EntityUUID,
		)
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			// The filesystem has already been removed.
			return nil
		} else if err != nil {
			return errors.Errorf(
				"checking filesystem %q is cross-provisioned: %w",
				job.EntityUUID, err,
			)
		}
		if canRemove {
			goto deleteFilesystem
		}
		return errors.Errorf(
			"filesystem %s status is not tombstone", job.EntityUUID,
		).Add(removalerrors.StorageFilesystemNoTombstone)
	}

deleteFilesystem:
	err = s.modelState.DeleteFilesystem(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf(
			"deleting filesystem %q: %w", job.EntityUUID, err,
		)
	}

	return nil
}

// RemoveDeadVolume is to be called from the storage provisoner to finally
// remove a dead volume that it has been gracefully cleaned up.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied volume UUID is not valid.
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// provided volume UUID.
// - [storageprovisioningerrors.VolumeNotDead] when the volume exists but was
// expected to be dead but was not.
func (s *Service) RemoveDeadVolume(
	ctx context.Context, uuid storageprovisioning.VolumeUUID,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := uuid.Validate()
	if err != nil {
		return errors.Errorf(
			"validating volume uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	fsLife, err := s.modelState.GetVolumeLife(ctx, uuid.String())
	if err != nil {
		return errors.Errorf("getting volume life for %q: %w", uuid, err)
	}
	if fsLife != life.Dead {
		return errors.Errorf(
			"volume %q is not dead", uuid,
		).Add(storageprovisioningerrors.VolumeNotDead)
	}

	tombstone := int(status.StorageVolumeStatusTypeTombstone)
	err = s.modelState.SetVolumeStatus(ctx, uuid.String(), tombstone)
	if err != nil {
		return errors.Errorf(
			"setting volume tombstone status for %q: %w", uuid, err,
		)
	}

	return nil
}

func (s *Service) volumeScheduleRemoval(
	ctx context.Context,
	volUUID storageprovisioning.VolumeUUID,
	force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	err = s.modelState.VolumeScheduleRemoval(
		ctx, jobUUID.String(),
		volUUID.String(), force, s.clock.Now().UTC().Add(wait),
	)
	if err != nil {
		return "", errors.Errorf("volume %q: %w", volUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for volume %q",
		jobUUID, volUUID)
	return jobUUID, nil
}

// processStorageVolumeRemovalJob handles the deletion of the volume.
// For a non-forced removal job, the volume is deleted once the life is dead and
// the status is tombstone. For forced removal job, deletion of a volume happens
// once the volume is no longer alive.
func (s *Service) processStorageVolumeRemovalJob(
	ctx context.Context, job removal.Job,
) error {
	if job.RemovalType != removal.StorageVolumeJob {
		return errors.Errorf(
			"job type: %q not valid for storage volume removal",
			job.RemovalType,
		).Add(removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetVolumeLife(ctx, job.EntityUUID)
	if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		// The volume has already been removed.
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting volume %q life: %w", job.EntityUUID, err,
		)
	}

	if l == life.Alive {
		return errors.Errorf(
			"volume %q is alive", job.EntityUUID,
		).Add(removalerrors.EntityStillAlive)
	} else if !job.Force && l == life.Dying {
		return errors.Errorf(
			"volume %q is not dead", job.EntityUUID,
		).Add(removalerrors.EntityNotDead)
	} else if !job.Force {
		sv, err := s.modelState.GetVolumeStatus(ctx, job.EntityUUID)
		if err != nil {
			return errors.Errorf(
				"getting volume %q status: %w", job.EntityUUID, err,
			)
		}
		if sv != int(status.StorageFilesystemStatusTypeTombstone) {
			return errors.Errorf(
				"volume %q status is not tombstone", job.EntityUUID,
			).Add(removalerrors.StorageVolumeNoTombstone)
		}
	}

	err = s.modelState.DeleteVolume(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf(
			"deleting volume %q: %w", job.EntityUUID, err,
		)
	}

	return nil
}

func (s *Service) filesystemAttachmentScheduleRemoval(
	ctx context.Context,
	fsaUUID storageprovisioning.FilesystemAttachmentUUID,
	force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	err = s.modelState.FilesystemAttachmentScheduleRemoval(
		ctx, jobUUID.String(), fsaUUID.String(),
		force, s.clock.Now().UTC().Add(wait),
	)
	if err != nil {
		return "", errors.Errorf("filesystem attachment %q: %w", fsaUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for filesystem attachment %q",
		jobUUID, fsaUUID)
	return jobUUID, nil
}

// processStorageFilesystemAttachmentRemovalJob handles the deletion of the
// filesystem attachment.
func (s *Service) processStorageFilesystemAttachmentRemovalJob(
	ctx context.Context, job removal.Job,
) error {
	if job.RemovalType != removal.StorageFilesystemAttachmentJob {
		return errors.Errorf(
			"job type: %q not valid for storage filesystem attachment removal",
			job.RemovalType,
		).Add(removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetFilesystemAttachmentLife(ctx, job.EntityUUID)
	if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
		// The filesystem attachment has already been removed.
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting filesystem attachment %q life: %w", job.EntityUUID, err,
		)
	}

	if l == life.Alive {
		return errors.Errorf(
			"filesystem attachment %q is alive", job.EntityUUID,
		).Add(removalerrors.EntityStillAlive)
	} else if !job.Force && l == life.Dying {
		return errors.Errorf(
			"filesystem attachment %q is not dead", job.EntityUUID,
		).Add(removalerrors.EntityNotDead)
	}

	err = s.modelState.DeleteFilesystemAttachment(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf(
			"deleting filesystem attachment %q: %w", job.EntityUUID, err,
		)
	}

	return nil
}

func (s *Service) volumeAttachmentScheduleRemoval(
	ctx context.Context,
	vaUUID storageprovisioning.VolumeAttachmentUUID,
	force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	err = s.modelState.VolumeAttachmentScheduleRemoval(
		ctx, jobUUID.String(), vaUUID.String(),
		force, s.clock.Now().UTC().Add(wait),
	)
	if err != nil {
		return "", errors.Errorf("volume attachment %q: %w", vaUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for volume attachment %q",
		jobUUID, vaUUID)
	return jobUUID, nil
}

// processStorageVolumeAttachmentRemovalJob handles the deletion of the
// volume attachment.
func (s *Service) processStorageVolumeAttachmentRemovalJob(
	ctx context.Context, job removal.Job,
) error {
	if job.RemovalType != removal.StorageVolumeAttachmentJob {
		return errors.Errorf(
			"job type: %q not valid for storage volume attachment removal",
			job.RemovalType,
		).Add(removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetVolumeAttachmentLife(ctx, job.EntityUUID)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
		// The volume attachment has already been removed.
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting volume attachment %q life: %w", job.EntityUUID, err,
		)
	}

	if l == life.Alive {
		return errors.Errorf(
			"volume attachment %q is alive", job.EntityUUID,
		).Add(removalerrors.EntityStillAlive)
	} else if !job.Force && l == life.Dying {
		return errors.Errorf(
			"volume attachment %q is not dead", job.EntityUUID,
		).Add(removalerrors.EntityNotDead)
	}

	err = s.modelState.DeleteVolumeAttachment(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf(
			"deleting volume attachment %q: %w", job.EntityUUID, err,
		)
	}

	return nil
}

func (s *Service) volumeAttachmentPlanScheduleRemoval(
	ctx context.Context,
	vapUUID storageprovisioning.VolumeAttachmentPlanUUID,
	force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	err = s.modelState.VolumeAttachmentPlanScheduleRemoval(
		ctx, jobUUID.String(), vapUUID.String(),
		force, s.clock.Now().UTC().Add(wait),
	)
	if err != nil {
		return "", errors.Errorf("volume attachment plan %q: %w", vapUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for volume attachment plan %q",
		jobUUID, vapUUID)
	return jobUUID, nil
}

// processStorageVolumeAttachmentPlanRemovalJob handles the deletion of the
// volume attachment.
func (s *Service) processStorageVolumeAttachmentPlanRemovalJob(
	ctx context.Context, job removal.Job,
) error {
	if job.RemovalType != removal.StorageVolumeAttachmentPlanJob {
		return errors.Errorf(
			"job type: %q not valid for storage volume attachment plan removal",
			job.RemovalType,
		).Add(removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetVolumeAttachmentPlanLife(ctx, job.EntityUUID)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
		// The volume attachment plan has already been removed.
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting volume attachment plan %q life: %w", job.EntityUUID, err,
		)
	}

	if l == life.Alive {
		return errors.Errorf(
			"volume attachment plan %q is alive", job.EntityUUID,
		).Add(removalerrors.EntityStillAlive)
	} else if !job.Force && l == life.Dying {
		return errors.Errorf(
			"volume attachment %q plan is not dead", job.EntityUUID,
		).Add(removalerrors.EntityNotDead)
	}

	err = s.modelState.DeleteVolumeAttachmentPlan(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf(
			"deleting volume attachment plan %q: %w", job.EntityUUID, err,
		)
	}

	return nil
}
