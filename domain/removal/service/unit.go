// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/machine"
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

// UnitState describes retrieval and persistence
// methods specific to unit removal.
type UnitState interface {
	// UnitExists returns true if a unit exists with the input unit UUID.
	UnitExists(ctx context.Context, unitUUID string) (bool, error)

	// EnsureUnitNotAliveCascade ensures that there is no unit identified by the
	// input unit UUID that is still "alive". If the unit is the last one on the
	// machine, the machine will also be guaranteed to not be "alive".
	// Unit storage attachments will be guaranteed not to be "alive", an if
	// destroyStorage is supplied as true, so will the unit's storage instances
	// All associated entities who's life advancement cascaded with the unit are
	// returned.
	EnsureUnitNotAliveCascade(
		ctx context.Context, unitUUID string, destroyStorage bool,
	) (internal.CascadedUnitLives, error)

	// GetRelationUnitsForUnit returns all relation-unit UUIDs for the input
	// unit UUID, thereby indicating what relations have this unit in their
	// scopes.
	GetRelationUnitsForUnit(ctx context.Context, unitUUID string) ([]string, error)

	// UnitScheduleRemoval schedules a removal job for the unit with the
	// input unit UUID, qualified with the input force boolean.
	UnitScheduleRemoval(ctx context.Context, removalUUID, unitUUID string, force bool, when time.Time) error

	// GetUnitLife returns the life of the unit with the input UUID.
	GetUnitLife(ctx context.Context, unitUUID string) (life.Life, error)

	// DeleteUnit removes a unit from the database completely.
	DeleteUnit(ctx context.Context, unitUUID string, force bool) error

	// GetApplicationNameAndUnitNameByUnitUUID retrieves the application name
	// and unit name for a unit identified by the input UUID. If the unit does
	// not exist, it returns an error.
	GetApplicationNameAndUnitNameByUnitUUID(ctx context.Context, unitUUID string) (string, string, error)

	// MarkUnitAsDead marks the unit with the input UUID as dead.
	MarkUnitAsDead(ctx context.Context, unitUUID string) error

	// GetCharmForUnit returns the charm UUID for the unit with the input unit UUID.
	// If the unit does not exist, it returns an empty string.
	GetCharmForUnit(ctx context.Context, unitUUID string) (string, error)
}

// RemoveUnit checks if a unit with the input name exists.
// If it does, the unit is guaranteed after this call to be:
// - Not alive.
// - Removed or scheduled to be removed with the input force qualification.
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// unit. This duration is ignored if the force argument is false.
// If the unit is the last one on the machine, the machine will be guaranteed
// to not be alive and be scheduled for removal.
// If destroyStorage is true, the unit's storage instances will be guaranteed
// to not be alive and be scheduled for removal.
// The UUID for the scheduled removal job is returned.
func (s *Service) RemoveUnit(
	ctx context.Context,
	unitUUID unit.UUID,
	destroyStorage bool,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.UnitExists(ctx, unitUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if unit exists: %w", err)
	} else if !exists {
		return "", errors.Errorf("unit does not exist").Add(applicationerrors.UnitNotFound)
	}

	// Ensure the unit is not alive. If it is the last one on the machine,
	// then we will return the machine UUID, which will be used to schedule
	// the removal of the machine.
	// If the machine UUID is returned, then the machine was also set to dying.
	cascaded, err := s.modelState.EnsureUnitNotAliveCascade(ctx, unitUUID.String(), destroyStorage)
	if err != nil {
		return "", errors.Errorf("unit %q: %w", unitUUID, err)
	}

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the unit if the normal destruction
			// workflows complete within the the wait duration.
			if _, err := s.unitScheduleRemoval(ctx, unitUUID, false, 0); err != nil {
				return "", errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal")
			wait = 0
		}
	}

	unitJobUUID, err := s.unitScheduleRemoval(ctx, unitUUID, force, wait)
	if err != nil {
		return "", errors.Capture(err)
	} else if cascaded.IsEmpty() {
		// No other entities associated with the unit were
		// also ensured to be "dying", so we're done.
		return unitJobUUID, nil
	}

	if cascaded.MachineUUID != nil {
		s.logger.Infof(ctx, "unit was the last one on machine %q, scheduling removal", *cascaded.MachineUUID)
		if _, err := s.machineScheduleRemoval(ctx, machine.UUID(*cascaded.MachineUUID), force, wait); err != nil {
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

	return unitJobUUID, nil
}

// MarkUnitAsDead marks the unit as dead. It will not remove the unit as
// that is a separate operation. This will advance the unit's life to dead
// and will not allow it to be transitioned back to alive.
// Returns an error if the unit does not exist.
func (s *Service) MarkUnitAsDead(ctx context.Context, unitUUID unit.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.UnitExists(ctx, unitUUID.String())
	if err != nil {
		return errors.Errorf("checking if unit exists: %w", err)
	} else if !exists {
		return errors.Errorf("unit does not exist").Add(applicationerrors.UnitNotFound)
	}

	return s.modelState.MarkUnitAsDead(ctx, unitUUID.String())
}

// leaveAllRelationScopes departs this unit from any relations that it is
// in-scope for. This must be called after the unit is dying, so we know it will
// not join any new relations from now on.
func (s *Service) leaveAllRelationScopes(ctx context.Context, unitUUID unit.UUID) error {
	relUnits, err := s.modelState.GetRelationUnitsForUnit(ctx, unitUUID.String())
	if err != nil {
		return errors.Errorf("getting relation scopes for unit %q: %w", unitUUID, err)
	}

	if len(relUnits) == 0 {
		return nil
	}
	s.logger.Infof(ctx, "unit %q departing relation scopes: %v", unitUUID, relUnits)

	for _, ru := range relUnits {
		if err := s.modelState.LeaveScope(ctx, ru); err != nil {
			return errors.Errorf("removing relation unit %q: %w", ru, err)
		}
	}

	return nil
}

func (s *Service) unitScheduleRemoval(
	ctx context.Context, unitUUID unit.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.modelState.UnitScheduleRemoval(
		ctx, jobUUID.String(), unitUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("unit: %w", err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for unit %q", jobUUID, unitUUID)
	return jobUUID, nil
}

// processUnitRemovalJob deletes a unit if it is dying.
// Note that we do not need transactionality here:
//   - Life can only advance - it cannot become alive if dying or dead.
func (s *Service) processUnitRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.UnitJob {
		return errors.Errorf("job type: %q not valid for unit removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetUnitLife(ctx, job.EntityUUID)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		// The unit has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("getting unit %q life: %w", job.EntityUUID, err)
	}

	// If the model is alive, we cannot delete it even with force. This is
	// programming error if we've reached this point and we're still alive.
	if l == life.Alive {
		return errors.Errorf("unit %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	if l == life.Dying && !job.Force {
		return errors.Errorf("unit %q is not dead", job.EntityUUID).Add(removalerrors.EntityNotDead)
	}

	// If we made it here, the unit is either dead, or we are processing a
	// forced removal. We cannot delete the unit record if the unit is still
	// in relation scopes. If the unit is dead, it transitioned to that
	// state itself without departing relations, and can not act in that
	// capacity again, so we depart all remaining scopes here.
	if err := s.leaveAllRelationScopes(ctx, unit.UUID(job.EntityUUID)); err != nil {
		return errors.Capture(err)
	}

	// The unit agent itself attempts to delete the unit's secrets,
	// but we must make sure here in the event that the agent is down.
	// A case has been made for the unit to create and update its own
	// secrets directly, but for deletion we could safely remove that
	// functionality and rely only on this code path.
	if err := s.deleteUnitOwnedSecrets(ctx, unit.UUID(job.EntityUUID)); err != nil {
		return errors.Capture(err)
	}

	charmUUID, err := s.modelState.GetCharmForUnit(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("getting charm for unit %q: %w", job.EntityUUID, err)
	}

	if err := s.modelState.DeleteUnit(ctx, job.EntityUUID, job.Force); errors.Is(err, applicationerrors.UnitNotFound) {
		// The unit has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("deleting unit: %w", err)
	}

	// Try to delete the charm if it is unused.
	if err := s.modelState.DeleteCharmIfUnused(ctx, charmUUID); err != nil {
		// Log the error but do not fail the removal job.
		s.logger.Warningf(ctx, "deleting charm for unit %q: %v", job.EntityUUID, err)
	}

	// If the unit was the leader of an application, we revoke leadership.
	// We do this last to expedite new leadership acquisition if the unit died
	// sooner that the expiry of its last lease.
	// For all other scenarios preventing lease renewal, the lease will be
	// relinquished naturally by expiry.
	applicationName, unitName, err := s.modelState.GetApplicationNameAndUnitNameByUnitUUID(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("getting application name and unit name: %w", err)
	}

	if err := s.leadershipRevoker.RevokeLeadership(applicationName, unit.Name(unitName)); err != nil && !errors.Is(err, leadership.ErrClaimNotHeld) {
		return errors.Errorf("revoking leadership: %w", err)
	}

	return nil
}
