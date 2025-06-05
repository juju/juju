// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// UnitState describes retrieval and persistence
// methods specific to unit removal.
type UnitState interface {
	// UnitExists returns true if a unit exists with the input unit UUID.
	UnitExists(ctx context.Context, unitUUID string) (bool, error)

	// EnsureUnitNotAlive ensures that there is no unit
	// identified by the input unit UUID, that is still alive.
	// If the unit is the last one on the machine, the machine is also set
	// to dying. The affected machine UUID is returned.
	EnsureUnitNotAlive(ctx context.Context, unitUUID string) (machineUUID string, err error)

	// UnitScheduleRemoval schedules a removal job for the unit with the
	// input unit UUID, qualified with the input force boolean.
	UnitScheduleRemoval(ctx context.Context, removalUUID, unitUUID string, force bool, when time.Time) error

	// GetUnitLife returns the life of the unit with the input UUID.
	GetUnitLife(ctx context.Context, unitUUID string) (life.Life, error)

	// DeleteUnit removes a unit from the database completely.
	DeleteUnit(ctx context.Context, unitUUID string) error

	// GetApplicationNameAndUnitNameByUnitUUID retrieves the application name
	// and unit name for a unit identified by the input UUID. If the unit does
	// not exist, it returns an error.
	GetApplicationNameAndUnitNameByUnitUUID(ctx context.Context, unitUUID string) (string, string, error)
}

// RemoveUnit checks if a unit with the input name exists.
// If it does, the unit is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// unit. This duration is ignored if the force argument is false.
// The UUID for the scheduled removal job is returned.
func (s *Service) RemoveUnit(
	ctx context.Context,
	unitUUID unit.UUID,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	exists, err := s.st.UnitExists(ctx, unitUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if unit exists: %w", err)
	} else if !exists {
		return "", errors.Errorf("unit does not exist").Add(applicationerrors.UnitNotFound)
	}

	// Ensure the unit is not alive. If it is the last one on the machine,
	// then we will return the machine UUID, which will be used to schedule
	// the removal of the machine.
	// If the machine UUID is returned, then the machine was also set to dying.
	machineUUID, err := s.st.EnsureUnitNotAlive(ctx, unitUUID.String())
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

		// TODO (stickupkid): Check that we don't have any storage attachments.
		// If we do, we need to schedule a removal job for the unit.
	}

	unitJobUUID, err := s.unitScheduleRemoval(ctx, unitUUID, force, wait)
	if err != nil {
		return "", errors.Capture(err)
	} else if machineUUID == "" {
		return unitJobUUID, nil
	}

	s.logger.Infof(ctx, "unit was the last one on machine %q, scheduling removal", machineUUID)

	// If the unit was the last one on the machine, we need to schedule
	// a removal job for the machine.
	if _, err := s.RemoveMachine(ctx, machine.UUID(machineUUID), force, wait); err != nil {
		// If the machine fails to be scheduled, then log out an error. The
		// units have been transitioned to dying and there is no way to
		// transition them back to alive.
		s.logger.Errorf(ctx, "failed to schedule removal of machine %q: %v", machineUUID, err)
	}

	return unitJobUUID, nil
}

func (s *Service) unitScheduleRemoval(
	ctx context.Context, unitUUID unit.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.st.UnitScheduleRemoval(
		ctx, jobUUID.String(), unitUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("unit: %w", err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q", jobUUID)
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

	l, err := s.st.GetUnitLife(ctx, job.EntityUUID)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		// The unit has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("getting unit %q life: %w", job.EntityUUID, err)
	}

	if l == life.Alive {
		return errors.Errorf("unit %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	// If the unit is the leader of an application, we need to revoke
	// leadership before we can delete it.
	applicationName, unitName, err := s.st.GetApplicationNameAndUnitNameByUnitUUID(ctx, job.EntityUUID)
	if err != nil {
		return errors.Errorf("getting application name and unit name: %w", err)
	}

	if err := s.leadershipRevoker.RevokeLeadership(applicationName, unit.Name(unitName)); err != nil && !errors.Is(err, leadership.ErrClaimNotHeld) {
		return errors.Errorf("revoking leadership: %w", err)
	}

	// Once we've removed leadership, we can delete the unit.
	if err := s.st.DeleteUnit(ctx, job.EntityUUID); errors.Is(err, applicationerrors.UnitNotFound) {
		// The unit has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("deleting unit: %w", err)
	}

	return nil
}
