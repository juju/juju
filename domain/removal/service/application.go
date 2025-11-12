// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

// ApplicationState describes retrieval and persistence
// methods specific to application removal.
type ApplicationState interface {
	// ApplicationExists returns true if a application exists with the input
	// application UUID.
	ApplicationExists(ctx context.Context, appUUID string) (bool, error)

	// EnsureApplicationNotAliveCascade ensures that there is no application
	// identified by the input application UUID, that is still alive. If the
	// application has units, they are also guaranteed to be no longer alive,
	// cascading. The affected unit UUIDs are returned. If the units are also
	// the last ones on their machines, it will cascade and the machines are
	// also set to dying. The affected machine UUIDs are returned.
	EnsureApplicationNotAliveCascade(
		ctx context.Context, appUUID string, destroyStorage, force bool,
	) (internal.CascadedApplicationLives, error)

	// ApplicationScheduleRemoval schedules a removal job for the application
	// with the input application UUID, qualified with the input force boolean.
	ApplicationScheduleRemoval(ctx context.Context, removalUUID, appUUID string, force bool, when time.Time) error

	// GetApplicationLife returns the life of the application with the input
	// UUID.
	GetApplicationLife(ctx context.Context, appUUID string) (life.Life, error)

	// DeleteApplication removes a application from the database completely.
	DeleteApplication(ctx context.Context, appUUID string, force bool) error

	// DeleteCharmIfUnused deletes the charm with the input UUID if it is not
	// used by any other application/unit.
	DeleteCharmIfUnused(ctx context.Context, charmUUID string) error

	// DeleteOrphanedResources deletes any resources associated with the input
	// charm UUID that are no longer referenced by any application.
	DeleteOrphanedResources(ctx context.Context, charmUUID string) error

	// GetChamrForApplication returns the charm UUID for the application with
	// the input application UUID.
	// If the application does not exist, it returns an empty string.
	GetCharmForApplication(ctx context.Context, appUUID string) (string, error)
}

// RemoveApplication checks if a application with the input application UUID
// exists. If it does, the application is guaranteed after this call to:
// - Not be alive.
// - Be removed or scheduled to be removed with the input force qualification.
// - Have no units that are alive.
// - Have all units scheduled for removal.
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// application. This duration is ignored if the force argument is false.
// If destroyStorage is true, the application units' storage instances will be
// guaranteed to be not alive and to be scheduled for removal.
// The UUID for the scheduled removal job is returned.
func (s *Service) RemoveApplication(
	ctx context.Context,
	appUUID coreapplication.UUID,
	destroyStorage bool,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.ApplicationExists(ctx, appUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if application %q exists: %w", appUUID, err)
	} else if !exists {
		return "", errors.Errorf("application %q does not exist", appUUID).Add(applicationerrors.ApplicationNotFound)
	}

	cascaded, err := s.modelState.EnsureApplicationNotAliveCascade(ctx, appUUID.String(), destroyStorage, force)
	if err != nil {
		return "", errors.Errorf("application %q: %w", appUUID, err)
	}

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the application if the normal destruction
			// workflows complete within the wait duration.
			if _, err := s.applicationScheduleRemoval(ctx, appUUID, false, 0); err != nil {
				return "", errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal of application %q", appUUID.String())
			wait = 0
		}
	}

	appJobUUID, err := s.applicationScheduleRemoval(ctx, appUUID, force, wait)
	if err != nil {
		return "", errors.Capture(err)
	}

	if cascaded.IsEmpty() {
		return appJobUUID, nil
	}

	for _, r := range cascaded.RelationUUIDs {
		if _, err := s.relationScheduleRemoval(ctx, relation.UUID(r), force, wait); err != nil {
			return "", errors.Capture(err)
		}
	}

	for _, u := range cascaded.UnitUUIDs {
		if _, err := s.unitScheduleRemoval(ctx, unit.UUID(u), force, wait); err != nil {
			return "", errors.Capture(err)
		}
	}

	for _, m := range cascaded.MachineUUIDs {
		if _, err := s.machineScheduleRemoval(ctx, machine.UUID(m), force, wait); err != nil {
			return "", errors.Capture(err)
		}
	}

	for _, a := range cascaded.StorageAttachmentUUIDs {
		if force && wait > 0 {
			if _, err := s.storageAttachmentScheduleRemoval(
				ctx, storageprovisioning.StorageAttachmentUUID(a), false, 0,
			); err != nil {
				return "", errors.Capture(err)
			}
		}
		if _, err := s.storageAttachmentScheduleRemoval(
			ctx, storageprovisioning.StorageAttachmentUUID(a), force, wait,
		); err != nil {
			return "", errors.Capture(err)
		}
	}

	for _, a := range cascaded.FileSystemAttachmentUUIDs {
		if force && wait > 0 {
			if _, err := s.filesystemAttachmentScheduleRemoval(
				ctx, storageprovisioning.FilesystemAttachmentUUID(a), false, 0,
			); err != nil {
				return "", errors.Capture(err)
			}
		}
		if _, err := s.filesystemAttachmentScheduleRemoval(
			ctx, storageprovisioning.FilesystemAttachmentUUID(a), force, wait,
		); err != nil {
			return "", errors.Capture(err)
		}
	}

	for _, a := range cascaded.VolumeAttachmentUUIDs {
		if force && wait > 0 {
			if _, err := s.volumeAttachmentScheduleRemoval(
				ctx, storageprovisioning.VolumeAttachmentUUID(a), false, 0,
			); err != nil {
				return "", errors.Capture(err)
			}
		}
		if _, err := s.volumeAttachmentScheduleRemoval(
			ctx, storageprovisioning.VolumeAttachmentUUID(a), force, wait,
		); err != nil {
			return "", errors.Capture(err)
		}
	}

	for _, a := range cascaded.VolumeAttachmentPlanUUIDs {
		if force && wait > 0 {
			if _, err := s.volumeAttachmentPlanScheduleRemoval(
				ctx, storageprovisioning.VolumeAttachmentPlanUUID(a), false, 0,
			); err != nil {
				return "", errors.Capture(err)
			}
		}
		if _, err := s.volumeAttachmentPlanScheduleRemoval(
			ctx, storageprovisioning.VolumeAttachmentPlanUUID(a), force, wait,
		); err != nil {
			return "", errors.Capture(err)
		}
	}

	for _, a := range cascaded.FileSystemUUIDs {
		if force && wait > 0 {
			if _, err := s.filesystemScheduleRemoval(
				ctx, storageprovisioning.FilesystemUUID(a), false, 0,
			); err != nil {
				return "", errors.Capture(err)
			}
		}
		if _, err := s.filesystemScheduleRemoval(
			ctx, storageprovisioning.FilesystemUUID(a), force, wait,
		); err != nil {
			return "", errors.Capture(err)
		}
	}

	for _, a := range cascaded.VolumeUUIDs {
		if force && wait > 0 {
			if _, err := s.volumeScheduleRemoval(
				ctx, storageprovisioning.VolumeUUID(a), false, 0,
			); err != nil {
				return "", errors.Capture(err)
			}
		}
		if _, err := s.volumeScheduleRemoval(
			ctx, storageprovisioning.VolumeUUID(a), force, wait,
		); err != nil {
			return "", errors.Capture(err)
		}
	}

	for _, a := range cascaded.StorageInstanceUUIDs {
		if force && wait > 0 {
			if _, err := s.storageInstanceScheduleRemoval(
				ctx, storage.StorageInstanceUUID(a), false, 0,
			); err != nil {
				return "", errors.Capture(err)
			}
		}
		if _, err := s.storageInstanceScheduleRemoval(
			ctx, storage.StorageInstanceUUID(a), force, wait,
		); err != nil {
			return "", errors.Capture(err)
		}
	}

	return appJobUUID, nil
}

func (s *Service) applicationScheduleRemoval(
	ctx context.Context, appUUID coreapplication.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.modelState.ApplicationScheduleRemoval(
		ctx, jobUUID.String(), appUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("application %q: %w", appUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for application %q", jobUUID, appUUID)
	return jobUUID, nil
}

// processApplicationRemovalJob deletes an application if it is dying.
// Note that we do not need transactionality here:
//   - Life can only advance - it cannot become alive if dying or dead.
func (s *Service) processApplicationRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.ApplicationJob {
		return errors.Errorf("job type: %q not valid for application removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetApplicationLife(ctx, job.EntityUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		// The application has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("getting application %q life: %w", job.EntityUUID, err)
	}

	// If the application is alive, we cannot delete it even with force. This is
	// programming error if we've reached this point and we're still alive.
	if l == life.Alive {
		return errors.Errorf("application %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	// The unit agent itself attempts to delete the applications's secrets if it
	// is the leader, but we must make sure here in the event that the agent is
	// down. A case has been made for the unit to create and update its own
	// secrets directly, but for deletion we could safely remove that
	// functionality and rely only on this code path.
	if err := s.deleteApplicationOwnedSecrets(ctx, coreapplication.UUID(job.EntityUUID)); err != nil {
		return errors.Capture(err)
	}

	// Get the CharmUUID before deleting the application, because after deletion
	// we won't be able to look it up.
	charmUUID, err := s.modelState.GetCharmForApplication(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("getting charm for application %q: %w", job.EntityUUID, err)
	}

	if err := s.modelState.DeleteApplication(ctx, job.EntityUUID, job.Force); errors.Is(err, applicationerrors.ApplicationNotFound) {
		// The application has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("deleting application %q: %w", job.EntityUUID, err)
	}

	// Try to delete any orphaned resources associated with the charm.
	if err := s.modelState.DeleteOrphanedResources(ctx, charmUUID); err != nil {
		// Log the error but do not fail the removal job.
		s.logger.Warningf(ctx, "deleting orphaned resources for application %q: %v", job.EntityUUID, err)
	}

	// Try to delete the charm if it is unused.
	if err := s.modelState.DeleteCharmIfUnused(ctx, charmUUID); err != nil {
		// Log the error but do not fail the removal job.
		s.logger.Warningf(ctx, "deleting charm for application %q: %v", job.EntityUUID, err)
	}

	return nil
}
