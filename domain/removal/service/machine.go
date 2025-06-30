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
	// GetMachineLife returns the life of the machine with the input UUID.
	GetMachineLife(ctx context.Context, mUUID string) (life.Life, error)
	// DeleteMachine deletes the specified machine and any dependent child records.
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
) (machine.UUID, error) {
	exists, err := s.st.MachineExists(ctx, machineUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if machine exists: %w", err)
	} else if !exists {
		return "", errors.Errorf("machine does not exist").Add(machineerrors.MachineNotFound)
	}

	// TODO (stickupkid): Finish the implementation with the machine epic.
	return "", nil
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
