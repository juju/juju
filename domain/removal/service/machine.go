// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/machine"
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

	// MarkMachineAsDead marks the machine with the input UUID as dead.
	MarkMachineAsDead(ctx context.Context, mUUID string) error

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
	unitUUIDs, machineUUIDs, err := s.st.EnsureMachineNotAliveCascade(ctx, machineUUID.String())
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
	} else if len(unitUUIDs) == 0 && len(machineUUIDs) == 0 {
		// If there are no units or machines to update, we can return early.
		return machineJobUUID, nil
	}

	// Ensure that the machines has units and child machines, which are removed
	// as well.
	if len(unitUUIDs) > 0 {
		// If there are any units that transitioned from alive to dying or dead,
		// we need to schedule their removal as well.
		s.logger.Infof(ctx, "machine has units %v, scheduling removal", unitUUIDs)

		s.removeUnits(ctx, unitUUIDs, force, wait)
	}

	if len(machineUUIDs) > 0 {
		// If there are any child machines that transitioned from alive to dying
		// or dead, we need to schedule their removal as well.
		s.logger.Infof(ctx, "machine has child machines %v, scheduling removal", machineUUIDs)

		s.removeMachines(ctx, machineUUIDs, force, wait)
	}

	return machineJobUUID, nil
}

// MarkMachineAsDead marks the machine as dead. It will not remove the machine as
// that is a separate operation. This will advance the machines's life to dead
// and will not allow it to be transitioned back to alive.
// Returns an error if the machine does not exist.
func (s *Service) MarkMachineAsDead(ctx context.Context, machineUUID machine.UUID) error {
	exists, err := s.st.MachineExists(ctx, machineUUID.String())
	if err != nil {
		return errors.Errorf("checking if machine exists: %w", err)
	} else if !exists {
		return errors.Errorf("machine does not exist").Add(machineerrors.MachineNotFound)
	}

	return s.st.MarkMachineAsDead(ctx, machineUUID.String())
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
