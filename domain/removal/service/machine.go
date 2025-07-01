// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// MachineState describes retrieval and persistence
// methods specific to machine removal.
type MachineState interface {
	// MachineExists returns true if a machine exists with the input machine
	// UUID.
	MachineExists(ctx context.Context, machineUUID string) (bool, error)

	// EnsureMachineNotAliveCascade ensures that there is no machine identified
	// by the input machine UUID, that is still alive.
	EnsureMachineNotAliveCascade(ctx context.Context, unitUUID string) (units, machines []string, err error)

	// MachineScheduleRemoval schedules a removal job for the machine with the
	// input UUID, qualified with the input force boolean.
	// We don't care if the unit does not exist at this point because:
	// - it should have been validated prior to calling this method,
	// - the removal job executor will handle that fact.
	MachineScheduleRemoval(
		ctx context.Context, removalUUID, machineUUID string, force bool, when time.Time,
	) error

	// GetMachineLife returns the life of the machine with the input UUID.
	GetMachineLife(ctx context.Context, mUUID string) (life.Life, error)

	// DeleteMachine deletes the specified machine and any dependent child
	// records.
	DeleteMachine(ctx context.Context, mName string) error
}

// RemoveMachine checks if a machine with the input name exists.
// If it does, the machine is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// machine. This duration is ignored if the force argument is false.
// The UUID for the scheduled removal job is returned.
func (s *Service) RemoveMachine(
	ctx context.Context,
	machineUUID machine.UUID,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	exists, err := s.st.MachineExists(ctx, machineUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if machine exists: %w", err)
	} else if !exists {
		return "", errors.Errorf("machine does not exist").Add(machineerrors.MachineNotFound)
	}

	// Ensure the machine is not alive.
	units, machines, err := s.st.EnsureMachineNotAliveCascade(ctx, machineUUID.String())
	if err != nil {
		return "", errors.Errorf("machine %q: %w", machineUUID, err)
	}

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the unit if the normal destruction
			// workflows complete within the the wait duration.
			if _, err := s.machineScheduleRemoval(ctx, machineUUID, false, 0); err != nil {
				return "", errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal")
			wait = 0
		}
	}

	machineJobUUID, err := s.machineScheduleRemoval(ctx, machineUUID, force, wait)
	if err != nil {
		return "", errors.Capture(err)
	} else if len(units) == 0 && len(machines) == 0 {
		// If there are no units or machines to update, we can return early.
		return machineJobUUID, nil
	}

	if len(units) > 0 {
		s.logger.Infof(ctx, "units were affected by machine removal %v, scheduling removal", units)

		// If there are units to update, we need to schedule their removal.
		for _, unitUUID := range units {
			if _, err := s.RemoveUnit(ctx, unit.UUID(unitUUID), force, wait); err != nil {
				// If the unit fails to be scheduled for removal, then log the
				// error. The machine has been transitioned to dying and there
				// is no way to transition it back to alive.
				s.logger.Errorf(ctx, "failed to schedule unit %q for removal: %v", unitUUID, err)
			}
		}
	}

	if len(machines) > 0 {
		s.logger.Infof(ctx, "child machines were affected by machine removal %v, scheduling removal", machines)

		// If there are child machines to update, we need to schedule their
		// removal.
		for _, mUUID := range machines {
			if _, err := s.RemoveMachine(ctx, machine.UUID(mUUID), force, wait); err != nil {
				// If the machine fails to be scheduled for removal, then log the
				// error. The machine has been transitioned to dying and there
				// is no way to transition it back to alive.
				s.logger.Errorf(ctx, "failed to schedule machine %q for removal: %v", mUUID, err)
			}
		}
	}

	return machineJobUUID, nil
}

func (s *Service) machineScheduleRemoval(
	ctx context.Context, machineUUID machine.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.st.MachineScheduleRemoval(
		ctx, jobUUID.String(), machineUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("unit: %w", err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q", jobUUID)
	return jobUUID, nil
}

// processMachineRemovalJob deletes an machine if it is dying.
// Note that we do not need transactionality here:
//   - Life can only advance - it cannot become alive if dying or dead.
func (s *Service) processMachineRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.MachineJob {
		return errors.Errorf("job type: %q not valid for machine removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.st.GetMachineLife(ctx, job.EntityUUID)
	if errors.Is(err, machineerrors.MachineNotFound) {
		// The machine has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("getting machine %q life: %w", job.EntityUUID, err)
	}

	if l == life.Alive {
		return errors.Errorf("machine %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	if err := s.st.DeleteMachine(ctx, job.EntityUUID); errors.Is(err, machineerrors.MachineNotFound) {
		// The machine has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("deleting machine %q: %w", job.EntityUUID, err)
	}
	return nil
}
