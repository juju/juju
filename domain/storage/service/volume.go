// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// VolumeService provides a sub service implementation for working with Volumes
// in the model.
type VolumeService struct {
	st VolumeState
}

// VolumeState describes the state layer interface required for getting Volume
// information in the model.
type VolumeState interface {
	// GetVolumeUUIDsByMachines returns all of the [domainstorage.VolumeUUID]s in
	// the model that are attached to at least one of the supplied
	// [coremachine.UUID]s.
	//
	// Should no Volumes be attached to any machine in the model an empty slice is
	// returned. As well as should an empty list of Machine UUIDs be supplied an
	// empty slice is returned with no error.
	//
	// The following errors may be returned:
	// - [domainmachineerrors.MachineNotFound] when one or more the supplied machine
	// uuids does not exist in the model.
	GetVolumeUUIDsByMachines(
		context.Context, []coremachine.UUID,
	) ([]domainstorage.VolumeUUID, error)
}

// GetVolumesByMachines returns a slice of VolumeUUIDs that are attached to one
// ore more of the supplied Machine UUIDs.
//
// If an empty list of Machine UUIDs is supplied the caller will get back an
// empty list of Volume UUIDs.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when one of the supplied
// Machine UUIDs is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when one or
// more of the supplied Machine UUIDs does not exist in the model.
func (s *VolumeService) GetVolumesByMachines(
	ctx context.Context, uuids []coremachine.UUID,
) ([]domainstorage.VolumeUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	for i, machineUUID := range uuids {
		if err := machineUUID.Validate(); err != nil {
			return nil, errors.Errorf(
				"machine uuid at index %d is not valid: %w", i, err,
			)
		}
	}

	volumeUUIDs, err := s.st.GetVolumeUUIDsByMachines(ctx, uuids)
	if err != nil {
		return nil, err
	}

	return volumeUUIDs, nil
}
