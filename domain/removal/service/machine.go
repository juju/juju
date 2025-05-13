// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/machine"
)

// MachineState describes retrieval and persistence
// methods specific to machine removal.
type MachineState interface{}

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
	// TODO (stickupkid): Implement when doing the machines epic.
	return "", nil
}
