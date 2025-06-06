// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// MachineState describes retrieval and persistence
// methods specific to machine removal.
type MachineState interface {
	// MachineExists returns true if a machine exists with the input machine
	// UUID.
	MachineExists(ctx context.Context, machineUUID string) (bool, error)
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
