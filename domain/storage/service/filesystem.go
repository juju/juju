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

// FilesystemService provides a sub service implementation for working with
// Filesystems in the model.
type FilesystemService struct {
	st FilesystemState
}

type FilesystemState interface {
	// GetFilesystemState returns all of the [domainstorage.FilesystemUUID]s in
	// the model that are attached to at least one of the supplied
	// [coremachine.UUID]s.
	//
	// Should no Filesystems be attached to any machine in the model an empty
	// slice is returned. As well as should an empty list of Machine UUIDs be
	// supplied an empty slice is returned with no error.
	//
	// The following errors may be returned:
	// - [domainmachineerrors.MachineNotFound] when one or more the supplied
	// machine uuids does not exist in the model.
	GetFilesystemUUIDsByMachines(
		context.Context, []coremachine.UUID,
	) ([]domainstorage.FilesystemUUID, error)
}

// GetFilesystemsByMachine returns a slice of FilesystemUUIDs that are attached
// to one ore more of the supplied Machine UUIDs.
//
// If an empty list of Machine UUIDs is supplied the caller will get back an
// empty list of Filesystem UUIDs.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when one of the supplied
// Machine UUIDs is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when one or
// more of the supplied Machine UUIDs does not exist in the model.
func (s *FilesystemService) GetFilesystemsByMachines(
	ctx context.Context,
	uuids []coremachine.UUID,
) ([]domainstorage.FilesystemUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	for i, machineUUID := range uuids {
		if err := machineUUID.Validate(); err != nil {
			return nil, errors.Errorf(
				"machine uuid at index %d is not valid: %w", i, err,
			)
		}
	}

	filesystemUUIDs, err := s.st.GetFilesystemUUIDsByMachines(ctx, uuids)
	if err != nil {
		return nil, err
	}

	return filesystemUUIDs, nil
}
