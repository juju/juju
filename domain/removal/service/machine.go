// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
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
	EnsureMachineNotAliveCascade(
		ctx context.Context, unitUUID string, force bool,
	) (internal.CascadedMachineLives, error)

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

	// GetInstanceLife returns the life of the machine instance with the input UUID.
	GetInstanceLife(ctx context.Context, mUUID string) (life.Life, error)

	// MarkMachineAsDead marks the machine with the input UUID as dead.
	MarkMachineAsDead(ctx context.Context, mUUID string) error

	// DeleteMachine deletes the specified machine and any dependent child
	// records.
	DeleteMachine(ctx context.Context, mName string, force bool) error

	// MarkInstanceAsDead marks the machine cloud instance with the input UUID as
	// dead.
	MarkInstanceAsDead(ctx context.Context, mUUID string) error

	// GetMachineNetworkInterfaces returns the network interfaces for the
	// machine with the input UUID. This is used to release any addresses
	// that container machine has allocated.
	GetMachineNetworkInterfaces(ctx context.Context, machineUUID string) ([]string, error)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.MachineExists(ctx, machineUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if machine exists: %w", err)
	} else if !exists {
		return "", errors.Errorf("machine does not exist").Add(machineerrors.MachineNotFound)
	}

	cascaded, err := s.modelState.EnsureMachineNotAliveCascade(ctx, machineUUID.String(), force)
	if err != nil {
		return "", errors.Errorf("machine %q: %w", machineUUID, err)
	}

	// Normalize the wait here so the cascaded scheduling below cannot
	// see a non-forced wait.
	wait = s.normalizeWait(ctx, machineUUID, force, wait)
	machineJobUUID, err := s.scheduleWithForceWait(ctx, machineUUID, force, wait, s.machineScheduleRemoval)
	if err != nil {
		return "", errors.Capture(err)
	}

	// If no other entities had their lives advanced to dying, we're done.
	if cascaded.IsEmpty() {
		return machineJobUUID, nil
	}

	if err := s.scheduleCascaded(ctx, cascaded.UnitUUIDs, force, wait, s.unitScheduleRemoval); err != nil {
		return "", errors.Capture(err)
	}

	if err := s.scheduleCascaded(ctx, cascaded.MachineUUIDs, force, wait, s.machineScheduleRemoval); err != nil {
		return "", errors.Capture(err)
	}

	if err := s.scheduleCascaded(ctx, cascaded.StorageAttachmentUUIDs, force, wait, s.storageAttachmentScheduleRemoval); err != nil {
		return "", errors.Capture(err)
	}

	if err := s.scheduleCascaded(ctx, cascaded.FileSystemAttachmentUUIDs, force, wait, s.filesystemAttachmentScheduleRemoval); err != nil {
		return "", errors.Capture(err)
	}

	if err := s.scheduleCascaded(ctx, cascaded.VolumeAttachmentUUIDs, force, wait, s.volumeAttachmentScheduleRemoval); err != nil {
		return "", errors.Capture(err)
	}

	if err := s.scheduleCascaded(ctx, cascaded.VolumeAttachmentPlanUUIDs, force, wait, s.volumeAttachmentPlanScheduleRemoval); err != nil {
		return "", errors.Capture(err)
	}

	if err := s.scheduleCascaded(ctx, cascaded.FileSystemUUIDs, force, wait, s.filesystemScheduleRemoval); err != nil {
		return "", errors.Capture(err)
	}

	if err := s.scheduleCascaded(ctx, cascaded.VolumeUUIDs, force, wait, s.volumeScheduleRemoval); err != nil {
		return "", errors.Capture(err)
	}

	if err := s.scheduleCascaded(ctx, cascaded.StorageInstanceUUIDs, force, wait, s.storageInstanceScheduleRemoval); err != nil {
		return "", errors.Capture(err)
	}

	return machineJobUUID, nil
}

// MarkMachineAsDead marks the machine as dead. It will not remove the machine as
// that is a separate operation. This will advance the machines's life to dead
// and will not allow it to be transitioned back to alive.
// The following errors are returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
// - [removalerrors.EntityStillAlive] if the machine is alive.
// - [removalerrors.MachineHasContainers] if the machine hosts containers.
// - [removalerrors.MachineHasUnits] if the machine hosts units.
// - [removalerrors.MachineHasStorage] if the machine hosts storage.
func (s *Service) MarkMachineAsDead(ctx context.Context, machineUUID machine.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.MachineExists(ctx, machineUUID.String())
	if err != nil {
		return errors.Errorf("checking if machine exists: %w", err)
	} else if !exists {
		return errors.Errorf("machine does not exist").Add(machineerrors.MachineNotFound)
	}

	return s.modelState.MarkMachineAsDead(ctx, machineUUID.String())
}

// MarkInstanceAsDead marks the machine's cloud instance as dead. It will not
// remove the instance as that is a separate operation. This will advance the
// instance's life to dead and will not allow it to be transitioned back to
// alive.
// The following errors are returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
// - [removalerrors.EntityStillAlive] if the machine is alive.
// - [removalerrors.MachineHasContainers] if the machine hosts containers.
// - [removalerrors.MachineHasUnits] if the machine hosts units.
func (s *Service) MarkInstanceAsDead(ctx context.Context, machineUUID machine.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.MachineExists(ctx, machineUUID.String())
	if err != nil {
		return errors.Errorf("checking if machine exists: %w", err)
	} else if !exists {
		return errors.Errorf("machine does not exist").Add(machineerrors.MachineNotFound)
	}

	return s.modelState.MarkInstanceAsDead(ctx, machineUUID.String())
}

func (s *Service) machineScheduleRemoval(
	ctx context.Context, machineUUID machine.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.modelState.MachineScheduleRemoval(
		ctx, jobUUID.String(), machineUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("machine: %w", err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for machine %q", jobUUID, machineUUID)
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

	l, err := s.modelState.GetMachineLife(ctx, job.EntityUUID)
	if errors.Is(err, machineerrors.MachineNotFound) {
		// The machine has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("getting machine %q life: %w", job.EntityUUID, err)
	}

	// The machine should never be alive, that is a programming error if it is.
	// The machine should either be dying or dead.
	if l == life.Alive {
		return errors.Errorf("machine %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	// If the job is not using force, we need to check that the machine is not
	// alive, and that the instance is dead. That way we don't delete machines
	// that are still transitioning or are in use.
	l, err = s.modelState.GetInstanceLife(ctx, job.EntityUUID)
	if err != nil && !errors.Is(err, machineerrors.MachineNotFound) {
		return errors.Errorf("getting instance %q life: %w", job.EntityUUID, err)
	}

	// The machine instance should never be alive, that is a programming error
	// if it is. The machine instance should either be dying or dead.
	if l == life.Alive {
		return errors.Errorf("machine instance %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	// This instance hasn't yet been marked as dead, so we will not delete it
	// yet if not forced. The removal job incomplete sentinel is chained so
	// the job is retried quietly by the removal worker, like the dependents
	// gates of DeleteMachine.
	if !job.Force && l != life.Dead {
		return errors.Errorf("machine instance %q is not dead", job.EntityUUID).
			Add(removalerrors.EntityNotDead).
			Add(removalerrors.RemovalJobIncomplete)
	}

	// Do this before we delete the machine, so that we can release any
	// addresses that the machine has allocated.
	if err := s.releaseContainerAddresses(ctx, job.EntityUUID, job.Force); err != nil {
		return errors.Errorf("releasing addresses for machine %q: %w", job.EntityUUID, err)
	}

	if err := s.modelState.DeleteMachine(ctx, job.EntityUUID, job.Force); errors.Is(err, machineerrors.MachineNotFound) {
		// The machine has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("deleting machine %q: %w", job.EntityUUID, err)
	}

	return nil
}

func (s *Service) releaseContainerAddresses(ctx context.Context, machineUUID string, force bool) error {
	// Get the provider for releasing the machine addresses. If the provider
	// does not support releasing addresses, we can return early.
	provider, err := s.providerGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return nil
	} else if err != nil {
		return errors.Errorf("getting provider: %w", err)
	}

	// Get all the machines network interfaces, so that we can release them
	// to the provider. This will only work on container machines.
	addresses, err := s.modelState.GetMachineNetworkInterfaces(ctx, machineUUID)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil
	} else if err != nil {
		return errors.Errorf("getting machine %q network interfaces: %w", machineUUID, err)
	}

	if len(addresses) == 0 {
		return nil
	}

	// If the provider supports the networking interface, but can't release
	// addresses, then we need to handle the NotSupported error gracefully.
	if err := provider.ReleaseContainerAddresses(ctx, addresses); errors.Is(err, coreerrors.NotSupported) {
		return nil
	} else if err != nil && !force {
		return errors.Errorf("releasing machine %q network interfaces: %w", machineUUID, err)
	} else if err != nil && force {
		s.logger.Warningf(ctx, "failed to release machine %q network interfaces: %v", machineUUID, err)
	}

	return nil
}
